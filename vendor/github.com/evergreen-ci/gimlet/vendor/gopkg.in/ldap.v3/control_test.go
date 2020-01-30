package ldap

import (
	"bytes"
	"fmt"
	"reflect"
	"runtime"
	"testing"

	ber "github.com/go-asn1-ber/asn1-ber"
)

func TestControlPaging(t *testing.T) {
	runControlTest(t, NewControlPaging(0))
	runControlTest(t, NewControlPaging(100))
}

func TestControlManageDsaIT(t *testing.T) {
	runControlTest(t, NewControlManageDsaIT(true))
	runControlTest(t, NewControlManageDsaIT(false))
}

func TestControlMicrosoftNotification(t *testing.T) {
	runControlTest(t, NewControlMicrosoftNotification())
}

func TestControlMicrosoftShowDeleted(t *testing.T) {
	runControlTest(t, NewControlMicrosoftShowDeleted())
}

func TestControlString(t *testing.T) {
	runControlTest(t, NewControlString("x", true, "y"))
	runControlTest(t, NewControlString("x", true, ""))
	runControlTest(t, NewControlString("x", false, "y"))
	runControlTest(t, NewControlString("x", false, ""))
}

func runControlTest(t *testing.T, originalControl Control) {
	header := ""
	if callerpc, _, line, ok := runtime.Caller(1); ok {
		if caller := runtime.FuncForPC(callerpc); caller != nil {
			header = fmt.Sprintf("%s:%d: ", caller.Name(), line)
		}
	}

	encodedPacket := originalControl.Encode()
	encodedBytes := encodedPacket.Bytes()

	// Decode directly from the encoded packet (ensures Value is correct)
	fromPacket, err := DecodeControl(encodedPacket)
	if err != nil {
		t.Errorf("%sdecoding encoded bytes control failed: %s", header, err)
	}
	if !bytes.Equal(encodedBytes, fromPacket.Encode().Bytes()) {
		t.Errorf("%sround-trip from encoded packet failed", header)
	}
	if reflect.TypeOf(originalControl) != reflect.TypeOf(fromPacket) {
		t.Errorf("%sgot different type decoding from encoded packet: %T vs %T", header, fromPacket, originalControl)
	}

	// Decode from the wire bytes (ensures ber-encoding is correct)
	pkt, err := ber.DecodePacketErr(encodedBytes)
	if err != nil {
		t.Errorf("%sdecoding encoded bytes failed: %s", header, err)
	}
	fromBytes, err := DecodeControl(pkt)
	if err != nil {
		t.Errorf("%sdecoding control failed: %s", header, err)
	}
	if !bytes.Equal(encodedBytes, fromBytes.Encode().Bytes()) {
		t.Errorf("%sround-trip from encoded bytes failed", header)
	}
	if reflect.TypeOf(originalControl) != reflect.TypeOf(fromPacket) {
		t.Errorf("%sgot different type decoding from encoded bytes: %T vs %T", header, fromBytes, originalControl)
	}
}

func TestDescribeControlManageDsaIT(t *testing.T) {
	runAddControlDescriptions(t, NewControlManageDsaIT(false), "Control Type (Manage DSA IT)")
	runAddControlDescriptions(t, NewControlManageDsaIT(true), "Control Type (Manage DSA IT)", "Criticality")
}

func TestDescribeControlPaging(t *testing.T) {
	runAddControlDescriptions(t, NewControlPaging(100), "Control Type (Paging)", "Control Value (Paging)")
	runAddControlDescriptions(t, NewControlPaging(0), "Control Type (Paging)", "Control Value (Paging)")
}

func TestDescribeControlMicrosoftNotification(t *testing.T) {
	runAddControlDescriptions(t, NewControlMicrosoftNotification(), "Control Type (Change Notification - Microsoft)")
}

func TestDescribeControlMicrosoftShowDeleted(t *testing.T) {
	runAddControlDescriptions(t, NewControlMicrosoftShowDeleted(), "Control Type (Show Deleted Objects - Microsoft)")
}

func TestDescribeControlString(t *testing.T) {
	runAddControlDescriptions(t, NewControlString("x", true, "y"), "Control Type ()", "Criticality", "Control Value")
	runAddControlDescriptions(t, NewControlString("x", true, ""), "Control Type ()", "Criticality")
	runAddControlDescriptions(t, NewControlString("x", false, "y"), "Control Type ()", "Control Value")
	runAddControlDescriptions(t, NewControlString("x", false, ""), "Control Type ()")
}

func runAddControlDescriptions(t *testing.T, originalControl Control, childDescriptions ...string) {
	header := ""
	if callerpc, _, line, ok := runtime.Caller(1); ok {
		if caller := runtime.FuncForPC(callerpc); caller != nil {
			header = fmt.Sprintf("%s:%d: ", caller.Name(), line)
		}
	}

	encodedControls := encodeControls([]Control{originalControl})
	addControlDescriptions(encodedControls)
	encodedPacket := encodedControls.Children[0]
	if len(encodedPacket.Children) != len(childDescriptions) {
		t.Errorf("%sinvalid number of children: %d != %d", header, len(encodedPacket.Children), len(childDescriptions))
	}
	for i, desc := range childDescriptions {
		if encodedPacket.Children[i].Description != desc {
			t.Errorf("%sdescription not as expected: %s != %s", header, encodedPacket.Children[i].Description, desc)
		}
	}

}
