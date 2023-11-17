package auth

import (
	"fmt"
	"os"

	"github.com/twilio/twilio-go"
	verify "github.com/twilio/twilio-go/rest/verify/v2"
)

func NewTwilioClient() *twilio.RestClient {
	TWILIO_ACCOUNT_SID := os.Getenv("TWILIO_ACCOUNT_SID")
	TWILIO_AUTH_TOKEN := os.Getenv("TWILIO_AUTH_TOKEN")
	return twilio.NewRestClientWithParams(twilio.ClientParams{
		Username: TWILIO_ACCOUNT_SID,
		Password: TWILIO_AUTH_TOKEN,
	})
}

// sends a verification code to the provided phone
// phone in format +535XXXXXXX
func SendOtp(tc *twilio.RestClient, to string) {
	TWILIO_VERIFY_SERVICE_SID := os.Getenv("TWILIO_VERIFY_SERVICE_SID")
	params := &verify.CreateVerificationParams{}
	params.SetTo(to)
	params.SetChannel("sms")

	resp, err := tc.VerifyV2.CreateVerification(TWILIO_VERIFY_SERVICE_SID, params)

	if err != nil {
		fmt.Println(err.Error())
	} else {
		fmt.Printf("Sent verification '%s'\n", *resp.Sid)
	}
}

// checks the code provided by the user
func CheckOtp(tc *twilio.RestClient, to string) {
	var code string
	fmt.Println("Please check your phone and enter the code:")
	fmt.Scanln(&code)

	TWILIO_VERIFY_SERVICE_SID := os.Getenv("TWILIO_VERIFY_SERVICE_SID")
	params := &verify.CreateVerificationCheckParams{}
	params.SetTo(to)
	params.SetCode(code)

	resp, err := tc.VerifyV2.CreateVerificationCheck(TWILIO_VERIFY_SERVICE_SID, params)

	if err != nil {
		fmt.Println(err.Error())
	} else if *resp.Status == "approved" {
		fmt.Println("Correct!")
	} else {
		fmt.Println("Incorrect!")
	}
}
