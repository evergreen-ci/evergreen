package scram

import (
	"crypto/hmac"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

type clientState int

const (
	clientStarting clientState = iota
	clientFirst
	clientFinal
	clientDone
)

// ClientConversation ...
type ClientConversation struct {
	client   *Client
	nonceGen NonceGeneratorFcn
	hashGen  HashGeneratorFcn
	minIters int
	state    clientState
	valid    bool
	gs2      string
	nonce    string
	c1b      string
	serveSig []byte
}

// Step ...
func (cc *ClientConversation) Step(challenge string) (response string, err error) {
	switch cc.state {
	case clientStarting:
		cc.state = clientFirst
		response, err = cc.firstMsg()
	case clientFirst:
		cc.state = clientFinal
		response, err = cc.finalMsg(challenge)
	case clientFinal:
		cc.state = clientDone
		response, err = cc.validateServer(challenge)
	default:
		response, err = "", errors.New("Conversation already completed")
	}
	return
}

// Done ...
func (cc *ClientConversation) Done() bool {
	return cc.state == clientDone
}

// Valid ...
func (cc *ClientConversation) Valid() bool {
	return cc.valid
}

func (cc *ClientConversation) firstMsg() (string, error) {
	// Values are cached for use in final message parameters
	cc.gs2 = cc.gs2Header()
	cc.nonce = cc.client.nonceGen()
	cc.c1b = fmt.Sprintf("n=%s,r=%s", encodeName(cc.client.username), cc.nonce)

	return cc.gs2 + cc.c1b, nil
}

func (cc *ClientConversation) finalMsg(s1 string) (string, error) {
	msg, err := parseServerFirst(s1)
	if err != nil {
		return "", err
	}

	// Check nonce prefix and update
	if !strings.HasPrefix(msg.nonce, cc.nonce) {
		return "", errors.New("server nonce did not extend client nonce")
	}
	cc.nonce = msg.nonce

	// Check iteration count vs minimum
	if msg.iters < cc.minIters {
		return "", fmt.Errorf("server requested too few iterations (%d)", msg.iters)
	}

	// Create client-final-message-without-proof
	c2wop := fmt.Sprintf(
		"c=%s,r=%s",
		base64.StdEncoding.EncodeToString([]byte(cc.gs2)),
		cc.nonce,
	)

	// Create auth message
	authMsg := cc.c1b + "," + s1 + "," + c2wop

	// Get derived keys from client cache
	dk := cc.client.GetDerivedKeys(KeyFactors{Salt: string(msg.salt), Iters: msg.iters})

	// Create proof as clientkey XOR clientsignature
	clientSignature := computeHMAC(cc.hashGen, dk.StoredKey, []byte(authMsg))
	clientProof := xorBytes(dk.ClientKey, clientSignature)
	proof := base64.StdEncoding.EncodeToString(clientProof)

	// Cache ServerSignature for later validation
	cc.serveSig = computeHMAC(cc.hashGen, dk.ServerKey, []byte(authMsg))

	return fmt.Sprintf("%s,p=%s", c2wop, proof), nil
}

func (cc *ClientConversation) validateServer(s2 string) (string, error) {
	msg, err := parseServerFinal(s2)
	if err != nil {
		return "", err
	}

	if len(msg.err) > 0 {
		return "", fmt.Errorf("server error: %s", msg.err)
	}

	if !hmac.Equal(msg.verifier, cc.serveSig) {
		return "", errors.New("server validation failed")
	}

	cc.valid = true
	return "", nil
}

func (cc *ClientConversation) gs2Header() string {
	if cc.client.authID == "" {
		return "n,,"
	}
	return fmt.Sprintf("n,%s,", encodeName(cc.client.authID))
}
