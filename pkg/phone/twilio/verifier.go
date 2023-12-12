package twilio

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	"github.com/twilio/twilio-go"
	"github.com/twilio/twilio-go/client"
	lookupsv1 "github.com/twilio/twilio-go/rest/lookups/v1"
	verifyv2 "github.com/twilio/twilio-go/rest/verify/v2"

	grpc_client "github.com/code-payments/code-server/pkg/grpc/client"
	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/phone"
)

const (
	metricsStructName = "phone.twilio.verifier"
)

var (
	androidAppHash = "+8j1B159Xfs"
)

const (
	// https://www.twilio.com/docs/api/errors/60200
	invalidParameterCode = 60200
	// https://www.twilio.com/docs/api/errors/60202
	maxCheckAttemptsCode = 60202
	// https://www.twilio.com/docs/api/errors/60203
	maxSendAttemptsCode = 60203
	// https://www.twilio.com/docs/api/errors/60220
	useCaseVettingCode = 60220
	// https://www.twilio.com/docs/api/errors/60410
	fraudDetectionCode = 60410
)

var (
	defaultChannel = "sms"
)

var (
	carrierMapKey           = "carrier"
	mobileCountryCodeMapKey = "mobile_country_code"
	mobileNetworkCodeMapKey = "mobile_network_code"
	phoneTypeMapKey         = "type"
)

var (
	statusApproved = "approved"
	statusCanceled = "canceled"
	statusPending  = "pending"
)

// https://www.twilio.com/docs/lookup/api#phone-number-type-values
var (
	phoneTypeLandline = "landline"
	phoneTypeMobile   = "mobile"
	phoneTypeVoip     = "voip"
)

type verifier struct {
	client     *twilio.RestClient
	serviceSid string
}

// NewVerifier returns a new phone verifier backed by Twilio
func NewVerifier(accountSid, serviceSid, authToken string) phone.Verifier {
	client := twilio.NewRestClientWithParams(twilio.ClientParams{
		Username: accountSid,
		Password: authToken,
	})

	return &verifier{
		client:     client,
		serviceSid: serviceSid,
	}
}

// SendCode implements phone.Verifier.SendCode
func (v *verifier) SendCode(ctx context.Context, phoneNumber string) (string, *phone.Metadata, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "SendCode")
	defer tracer.End()

	err := v.checkValidPhoneNumber(phoneNumber)
	if err != nil {
		tracer.OnError(err)
		return "", nil, err
	}

	var appHash *string
	userAgent, err := grpc_client.GetUserAgent(ctx)
	if err == nil && userAgent.DeviceType == grpc_client.DeviceTypeAndroid {
		appHash = &androidAppHash
	}

	resp, err := v.client.VerifyV2.CreateVerification(v.serviceSid, &verifyv2.CreateVerificationParams{
		To:      &phoneNumber,
		Channel: &defaultChannel,
		AppHash: appHash,
	})
	if err != nil {
		err = checkInvalidToParameterError(err, phone.ErrInvalidNumber)
		err = checkMaxSendAttemptsError(err, phone.ErrRateLimited)
		err = checkFraudDetectionError(err, phone.ErrRateLimited)
		err = checkUseCaseVettingError(err, phone.ErrRateLimited)
		tracer.OnError(err)
		return "", nil, err
	}

	if resp.Sid == nil {
		err = errors.New("sid not provided")
		tracer.OnError(err)
		return "", nil, err
	}

	metadata := getMetadataFromLookupMap(phoneNumber, resp.Lookup)

	return *resp.Sid, metadata, nil
}

// Check implements phone.Verifier.Check
func (v *verifier) Check(ctx context.Context, phoneNumber, code string) error {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "Check")
	defer tracer.End()

	if !phone.IsVerificationCode(code) {
		err := phone.ErrInvalidVerificationCode
		tracer.OnError(err)
		return err
	}

	resp, err := v.client.VerifyV2.CreateVerificationCheck(v.serviceSid, &verifyv2.CreateVerificationCheckParams{
		To:   &phoneNumber,
		Code: &code,
	})
	if err != nil {
		err = check404Error(err, phone.ErrNoVerification)
		err = checkMaxCheckAttemptsError(err, phone.ErrNoVerification)
		tracer.OnError(err)
		return err
	}

	if resp.Status == nil {
		err = errors.New("status not provided")
		tracer.OnError(err)
		return err
	}

	switch strings.ToLower(*resp.Status) {
	case statusApproved:
		err = nil
	case statusCanceled:
		err = phone.ErrNoVerification
	default:
		err = phone.ErrInvalidVerificationCode
	}
	tracer.OnError(err)
	return err
}

// Cancel implements phone.Verifier.Cancel
func (v *verifier) Cancel(ctx context.Context, id string) error {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "Cancel")
	defer tracer.End()

	_, err := v.client.VerifyV2.UpdateVerification(v.serviceSid, id, &verifyv2.UpdateVerificationParams{
		Status: &statusCanceled,
	})

	if err != nil {
		err := check404Error(err, nil)
		tracer.OnError(err)
		return err
	}

	return nil
}

// IsVerificationActive implements phone.Verifier.IsVerificationActive
func (v *verifier) IsVerificationActive(ctx context.Context, id string) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "IsVerificationActive")
	defer tracer.End()

	resp, err := v.client.VerifyV2.FetchVerification(v.serviceSid, id)
	if is404Error(err) {
		return false, nil
	} else if err != nil {
		tracer.OnError(err)
		return false, err
	}

	if resp.Status == nil {
		err = errors.New("status is not provided")
		tracer.OnError(err)
		return false, err
	}

	return *resp.Status == statusPending, nil
}

// IsValidPhoneNumber implements phone.Verifier.IsValidPhoneNumber
func (v *verifier) IsValidPhoneNumber(ctx context.Context, phoneNumber string) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "IsValidPhoneNumber")
	defer tracer.End()

	err := v.checkValidPhoneNumber(phoneNumber)
	if err == phone.ErrInvalidNumber || err == phone.ErrUnsupportedPhoneType {
		return false, nil
	} else if err != nil {
		tracer.OnError(err)
		return false, err
	}
	return true, nil
}

// todo: Use the new V2 API when we get access. It has some cool features like
// SIM swap detection.
func (v *verifier) checkValidPhoneNumber(phoneNumber string) error {
	if !phone.IsE164Format(phoneNumber) {
		return errors.New("phone number is not in E.164 format")
	}

	resp, err := v.client.LookupsV1.FetchPhoneNumber(phoneNumber, &lookupsv1.FetchPhoneNumberParams{
		Type: &[]string{carrierMapKey},
	})
	if err != nil {
		return check404Error(err, phone.ErrInvalidNumber)
	}

	if resp.Carrier == nil {
		return nil
	}

	carrierInfoMap, ok := (*resp.Carrier).(map[string]interface{})
	if !ok {
		return nil
	}
	metadata := getMetadataFromCarrierInfoMap(phoneNumber, carrierInfoMap)

	// This is not guaranteed to be available
	if metadata.Type == nil {
		return nil
	}

	// todo: It's safest to block all virtual numbers, but confirm with Twilio
	//       if there are cases where we'd want this to go through.
	if *metadata.Type != phone.TypeMobile {
		return phone.ErrUnsupportedPhoneType
	}
	return nil
}

func check404Error(inError, outError error) error {
	if is404Error(inError) {
		return outError
	}
	return inError
}

func is404Error(err error) bool {
	twilioError, ok := err.(*client.TwilioRestError)
	if !ok {
		return false
	}

	return twilioError.Status == http.StatusNotFound
}

func checkMaxSendAttemptsError(inError, outError error) error {
	twilioError, ok := inError.(*client.TwilioRestError)
	if !ok {
		return inError
	}

	if twilioError.Status != http.StatusTooManyRequests {
		return inError
	}

	if twilioError.Code == maxSendAttemptsCode {
		return outError
	}
	return inError
}

func checkMaxCheckAttemptsError(inError, outError error) error {
	twilioError, ok := inError.(*client.TwilioRestError)
	if !ok {
		return inError
	}

	if twilioError.Status != http.StatusTooManyRequests {
		return inError
	}

	if twilioError.Code == maxCheckAttemptsCode {
		return outError
	}
	return inError
}

func checkInvalidToParameterError(inError, outError error) error {
	twilioError, ok := inError.(*client.TwilioRestError)
	if !ok {
		return inError
	}

	if twilioError.Status != http.StatusBadRequest {
		return inError
	}

	if twilioError.Code != invalidParameterCode {
		return inError
	}

	expectedMessage := strings.ToLower("Invalid parameter `To`")
	if strings.Contains(strings.ToLower(twilioError.Message), expectedMessage) {
		return outError
	}
	return inError
}

func checkUseCaseVettingError(inError, outError error) error {
	twilioError, ok := inError.(*client.TwilioRestError)
	if !ok {
		return inError
	}

	if twilioError.Code == useCaseVettingCode {
		return outError
	}
	return inError
}

func checkFraudDetectionError(inError, outError error) error {
	twilioError, ok := inError.(*client.TwilioRestError)
	if !ok {
		return inError
	}

	if twilioError.Code == fraudDetectionCode {
		return outError
	}
	return inError
}

// I'm soo sorry about the below code. The Twilio client is ridiculous with typing.

// Note: Can't use "ok" variable because it appears to actually set a key with a nil
//       value

func getMetadataFromLookupMap(phoneNumber string, lookup *interface{}) *phone.Metadata {
	metadata := &phone.Metadata{
		PhoneNumber: phoneNumber,
	}

	if lookup == nil {
		return metadata
	}

	carrierInfo := (*lookup).(map[string]interface{})[carrierMapKey]
	if carrierInfo == nil {
		return metadata
	}

	carrierInfoMap, ok := carrierInfo.(map[string]interface{})
	if !ok {
		return metadata
	}

	return getMetadataFromCarrierInfoMap(phoneNumber, carrierInfoMap)
}

func getMetadataFromCarrierInfoMap(phoneNumber string, carrierInfo map[string]interface{}) *phone.Metadata {
	metadata := &phone.Metadata{
		PhoneNumber: phoneNumber,
	}

	if carrierInfo == nil {
		return metadata
	}

	phoneType := carrierInfo[phoneTypeMapKey]
	if phoneType != nil {
		strValue, ok := phoneType.(string)
		if ok {
			metadata.SetType(getPhoneTypeFromString(strValue))
		}
	}

	mobileCountryCode := carrierInfo[mobileCountryCodeMapKey]
	if mobileCountryCode != nil {
		strValue, ok := mobileCountryCode.(string)
		if ok {
			intValue, err := strconv.Atoi(strValue)
			if err == nil {
				metadata.SetMobileCountryCode(intValue)
			}
		}
	}

	mobileNetworkCode := carrierInfo[mobileNetworkCodeMapKey]
	if mobileNetworkCode != nil {
		strValue, ok := mobileNetworkCode.(string)
		if ok {
			intValue, err := strconv.Atoi(strValue)
			if err == nil {
				metadata.SetMobileNetworkCode(intValue)
			}
		}
	}

	return metadata
}

func getPhoneTypeFromString(value string) phone.Type {
	switch value {
	case phoneTypeMobile:
		return phone.TypeMobile
	case phoneTypeVoip:
		return phone.TypeVoip
	case phoneTypeLandline:
		return phone.TypeLandline
	default:
		return phone.TypeUnknown
	}
}
