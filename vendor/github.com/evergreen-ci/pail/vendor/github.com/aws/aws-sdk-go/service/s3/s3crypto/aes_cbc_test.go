package s3crypto

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"testing"
)

func TestAESCBCEncryptDecrypt(t *testing.T) {
	var testCases = []struct {
		key        string
		iv         string
		plaintext  string
		ciphertext string
		decodeHex  bool
		padder     Padder
	}{
		// Test vectors from RFC 3602: https://tools.ietf.org/html/rfc3602
		{
			"06a9214036b8a15b512e03d534120006",
			"3dafba429d9eb430b422da802c9fac41",
			"Single block msg",
			"e353779c1079aeb82708942dbe77181a",
			false,
			NoPadder,
		},
		{
			"c286696d887c9aa0611bbb3e2025a45a",
			"562e17996d093d28ddb3ba695a2e6f58",
			"000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
			"d296cd94c2cccf8a3a863028b5e1dc0a7586602d253cfff91b8266bea6d61ab1",
			true,
			NoPadder,
		},
		{
			"6c3ea0477630ce21a2ce334aa746c2cd",
			"c782dc4c098c66cbd9cd27d825682c81",
			"This is a 48-byte message (exactly 3 AES blocks)",
			"d0a02b3836451753d493665d33f0e8862dea54cdb293abc7506939276772f8d5021c19216bad525c8579695d83ba2684",
			false,
			NoPadder,
		},
		{
			"56e47a38c5598974bc46903dba290349",
			"8ce82eefbea0da3c44699ed7db51b7d9",
			"a0a1a2a3a4a5a6a7a8a9aaabacadaeafb0b1b2b3b4b5b6b7b8b9babbbcbdbebfc0c1c2c3c4c5c6c7c8c9cacbcccdcecfd0d1d2d3d4d5d6d7d8d9dadbdcdddedf",
			"c30e32ffedc0774e6aff6af0869f71aa0f3af07a9a31a9c684db207eb0ef8e4e35907aa632c3ffdf868bb7b29d3d46ad83ce9f9a102ee99d49a53e87f4c3da55",
			true,
			NoPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"",
			"B012949BA07D1A6DCE9DEE67274D41AB",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"41",
			"8A11ABA68A566132FFE04DB336621D41",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"4141",
			"97D0896E41DFDB5CEA4A9EB70A938CFD",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"414141",
			"8464EAD45FA2D8790E8741E32C28083F",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"41414141",
			"1E656D6E2745BA9F154FAF136B2BC73D",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"4141414141",
			"0B6031C4B230DAC6BD6D3F195645B287",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"414141414141",
			"5D09FEB6462BB489489A7E18FD341D9D",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"41414141414141",
			"85745E398F2FD1050C2CE8F8614DA369",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"4141414141414141",
			"7BE52933970BA7B0FC6FB3FC37648205",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"414141414141414141",
			"ED3A1E134EF36CCFE60C8123B4272F89",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"41414141414141414141",
			"C3B7C9E177E1052FC736F65FC1E74209",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"4141414141414141414141",
			"C3A8B53F7F57F0B9D346FA99810A3C28",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"414141414141414141414141",
			"D16B1ECE5BF00AF919E139E99775FF06",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"41414141414141414141414141",
			"B258F4DF57FFCA1EFCF8D76140F05139",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"4141414141414141414141414141",
			"3CD2282DE24A2CF9E23326CC3DC9077A",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"414141414141414141414141414141",
			"3010232E7C752A3B4C9EE428B4C4FE88",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"41414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217A22BC4E6D03BFD2418DD412D1ED1B31AF",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"4141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217A5427BBD4A4D50776989441370E3B5B16",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"414141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217A7FF985F55567D1B25EA40E23BB4CB1FE",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"41414141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217A0835E548C7370D8F8D9925C0E6B54727",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"4141414141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217ADC0CF1436399E67BC1122B31CB596649",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"414141414141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217A3D096F0DEAFF91938B82E5D404B0B065",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"41414141414141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217AD56ABA897A355CF307CCB74226243192",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"4141414141414141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217A151284F950B1B1DBCAD6D9E7900DF4E6",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"414141414141414141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217AEF85A612514121C299A1D87116C4A182",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"41414141414141414141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217A67F157569BFB4013EA3AD16DB8C69AD6",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"4141414141414141414141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217AF8520D191F6ACBD88B2140588B91C697",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"414141414141414141414141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217ADD8BBAA71745669B96F2683E2F5AEC35",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"41414141414141414141414141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217AFB2D4282817D7EC6B33EFAD7AA14A3C5",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"4141414141414141414141414141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217A459B89E7E0DAF3DA654576B60B2DA7CE",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"414141414141414141414141414141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217A65759F23F9789D05B23D5DBAA9E32036",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"41414141414141414141414141414141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217A03C78FBD5E2CB08B3B6D181E23FBDE79",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"4141414141414141414141414141414141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217ACEF79EE1163A2F52888F87A3979EB3CA013D941FBBDE56C106C482CD022F290F",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"414141414141414141414141414141414141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217ACEF79EE1163A2F52888F87A3979EB3CA0645D313AC3C29B79DB1AA2E00A5B393",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"41414141414141414141414141414141414141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217ACEF79EE1163A2F52888F87A3979EB3CA2ED0FD8048053BF22EBE501D82C4B3F1",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"4141414141414141414141414141414141414141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217ACEF79EE1163A2F52888F87A3979EB3CAC57D706C7866A01D6E913F98AE57EE54",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"414141414141414141414141414141414141414141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217ACEF79EE1163A2F52888F87A3979EB3CAB7FC1241FAFDFE45C4FF982D5DC1DAEF",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"41414141414141414141414141414141414141414141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217ACEF79EE1163A2F52888F87A3979EB3CA7063EA296922DE8BDFD3B29D786C5F91",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"4141414141414141414141414141414141414141414141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217ACEF79EE1163A2F52888F87A3979EB3CA3A4603475F4AFDBFADC6E7FA908188B1",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"414141414141414141414141414141414141414141414141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217ACEF79EE1163A2F52888F87A3979EB3CA3365C63C2AF2A6C8FB4D0E9ED3C6FDA3",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"41414141414141414141414141414141414141414141414141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217ACEF79EE1163A2F52888F87A3979EB3CA78BCC1874C0B7EB52645FC8F03B9C9CF",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"4141414141414141414141414141414141414141414141414141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217ACEF79EE1163A2F52888F87A3979EB3CA9B7A31397718EECB89B9E9CCCD729326",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"414141414141414141414141414141414141414141414141414141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217ACEF79EE1163A2F52888F87A3979EB3CAB15EA8A67E9E9FADB4249710277F3D4F",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"41414141414141414141414141414141414141414141414141414141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217ACEF79EE1163A2F52888F87A3979EB3CA94641D6A076193C660632CEA3F9CB02C",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"4141414141414141414141414141414141414141414141414141414141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217ACEF79EE1163A2F52888F87A3979EB3CAB2170A08417BE77F0EAA9110F4790E12",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"414141414141414141414141414141414141414141414141414141414141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217ACEF79EE1163A2F52888F87A3979EB3CA4E30F1CD7B2256ABD57DC3DAB05376C9",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"41414141414141414141414141414141414141414141414141414141414141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217ACEF79EE1163A2F52888F87A3979EB3CA9909B7B93D01BDAAC22D15AF34DF1EEF",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"4141414141414141414141414141414141414141414141414141414141414141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217ACEF79EE1163A2F52888F87A3979EB3CAD97F5D1206F00E5C7225CAD81CCD4027",
			true,
			AESCBCPadder,
		},
		{
			"11111111111111111111111111111111",
			"22222222222222222222222222222222",
			"414141414141414141414141414141414141414141414141414141414141414141414141414141414141414141414141",
			"C3304FA46097CBBA59085416764A217ACEF79EE1163A2F52888F87A3979EB3CA570CBB001A0C87558906B60C884AB5F41DA97CEF2A9401BC6DD0D22A54DBAD6D",
			true,
			AESCBCPadder,
		},
	}

	for i, testCase := range testCases {
		key, _ := hex.DecodeString(testCase.key)
		iv, _ := hex.DecodeString(testCase.iv)
		cd := CipherData{
			Key: key,
			IV:  iv,
		}

		cbc, err := newAESCBC(cd, testCase.padder)
		if err != nil {
			t.Fatal(fmt.Sprintf("Case %d: Expected no error for cipher creation, but received: %v", i, err.Error()))
		}

		plaintext := []byte(testCase.plaintext)
		if testCase.decodeHex {
			plaintext, _ = hex.DecodeString(testCase.plaintext)
		}

		cipherdata := cbc.Encrypt(bytes.NewReader(plaintext))
		ciphertext := []byte{}
		b := make([]byte, 19)
		err = nil
		n := 0
		for err != io.EOF {
			n, err = cipherdata.Read(b)
			ciphertext = append(ciphertext, b[:n]...)
		}

		if err != io.EOF {
			t.Fatal(fmt.Sprintf("Case %d: Expected no error during io reading, but received: %v", i, err.Error()))
		}

		expectedData, _ := hex.DecodeString(testCase.ciphertext)
		if bytes.Compare(expectedData, ciphertext) != 0 {
			t.Log("\n", ciphertext, "\n", expectedData)
			t.Fatal(fmt.Sprintf("Case %d: AES CBC encryption fails. Data is not the same", i))
		}

		plaindata := cbc.Decrypt(bytes.NewReader(ciphertext))
		plaintextDecrypted := []byte{}
		err = nil
		for err != io.EOF {
			n, err = plaindata.Read(b)
			plaintextDecrypted = append(plaintextDecrypted, b[:n]...)
		}
		if err != io.EOF {
			t.Fatal(fmt.Sprintf("Case %d: Expected no error during io reading, but received: %v", i, err.Error()))
		}

		if bytes.Compare(plaintext, plaintextDecrypted) != 0 {
			t.Log("\n", plaintext, "\n", plaintextDecrypted)
			t.Fatal(fmt.Sprintf("Case %d: AES CBC decryption fails. Data is not the same", i))
		}
	}
}
