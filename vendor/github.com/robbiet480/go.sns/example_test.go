package sns_test

import (
	"encoding/json"
	"fmt"

	"github.com/robbiet480/go.sns"
)

var notificationJson = ""

func ExamplePayload_VerifyPayload() {
	var notificationPayload sns.Payload
	err := json.Unmarshal([]byte(notificationJson), &notificationPayload)
	if err != nil {
		fmt.Print(err)
	}
	verifyErr := notificationPayload.VerifyPayload()
	if verifyErr != nil {
		fmt.Print(verifyErr)
	} else {
		fmt.Print("Payload is valid!")
	}
}

func ExamplePayload_Subscribe() {
	var notificationPayload sns.Payload
	err := json.Unmarshal([]byte(notificationJson), &notificationPayload)
	if err != nil {
		fmt.Print(err)
	}
	subscriptionResponse, err := notificationPayload.Subscribe()
	if err != nil {
		fmt.Println("Error when subscribing!", err)
	}
	fmt.Printf("subscriptionResponse %+v", subscriptionResponse)
}

func ExamplePayload_Unsubscribe() {
	var notificationPayload sns.Payload
	err := json.Unmarshal([]byte(notificationJson), &notificationPayload)
	if err != nil {
		fmt.Print(err)
	}
	unsubscriptionResponse, err := notificationPayload.Unsubscribe()
	if err != nil {
		fmt.Println("Error when unsubscribing!", err)
	}
	fmt.Printf("unsubscriptionResponse %+v", unsubscriptionResponse)
}
