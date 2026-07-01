package model

import (
	"strconv"
	"time"
)

// type metricTime time.Time
//
//	func (t metricTime) MarshalJSON() ([]byte, error) {
//		// Emit an ISO8601 timestamp so logs are human-readable.
//		return json.Marshal(time.Time(t).UTC().Format(time.RFC3339Nano))
//	}
//
//	func (t *metricTime) UnmarshalJSON(data []byte) error {
//		raw := strings.TrimSpace(string(data))
//		if raw == "" || raw == "null" {
//			return nil
//		}
//
//		n, err := strconv.ParseInt(raw, 10, 64)
//		if err != nil {
//			return fmt.Errorf("invalid metric time: %w", err)
//		}
//		*t = metricTime(time.UnixMilli(n).UTC())
//		return nil
//	}
//type metricTime int64

// https://github.com/aws/aws-sdk-go/blob/070853e88d22854d2355c2543d0958a5f76ad407/aws/csm/metric.go#L10C1-L15C2
type MetricTime time.Time

func (t MetricTime) MarshalJSON() ([]byte, error) {
	ns := time.Duration(time.Time(t).UnixNano())
	return []byte(strconv.FormatInt(int64(ns/time.Millisecond), 10)), nil
}

func (t *MetricTime) UnmarshalJSON(data []byte) error {
	raw := string(data)
	if raw == "" || raw == "null" {
		return nil
	}

	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return err
	}
	*t = MetricTime(time.UnixMilli(n).UTC())
	return nil
}

type Event struct {
	ClientID  *string     `json:"ClientId,omitempty"`
	API       *string     `json:"Api,omitempty"`
	Service   *string     `json:"Service,omitempty"`
	Timestamp *MetricTime `json:"Timestamp,omitempty"`
	Type      *string     `json:"Type,omitempty"`
	Version   *int        `json:"Version,omitempty"`

	AttemptCount *int `json:"AttemptCount,omitempty"`
	Latency      *int `json:"Latency,omitempty"`

	Fqdn           *string `json:"Fqdn,omitempty"`
	UserAgent      *string `json:"UserAgent,omitempty"`
	AttemptLatency *int    `json:"AttemptLatency,omitempty"`

	SessionToken   *string `json:"SessionToken,omitempty"`
	Region         *string `json:"Region,omitempty"`
	AccessKey      *string `json:"AccessKey,omitempty"`
	HTTPStatusCode *int    `json:"HttpStatusCode,omitempty"`
	XAmzID2        *string `json:"XAmzId2,omitempty"`
	XAmzRequestID  *string `json:"XAmznRequestId,omitempty"`

	AWSException        *string `json:"AwsException,omitempty"`
	AWSExceptionMessage *string `json:"AwsExceptionMessage,omitempty"`
	SDKException        *string `json:"SdkException,omitempty"`
	SDKExceptionMessage *string `json:"SdkExceptionMessage,omitempty"`

	FinalHTTPStatusCode      *int    `json:"FinalHttpStatusCode,omitempty"`
	FinalAWSException        *string `json:"FinalAwsException,omitempty"`
	FinalAWSExceptionMessage *string `json:"FinalAwsExceptionMessage,omitempty"`
	FinalSDKException        *string `json:"FinalSdkException,omitempty"`
	FinalSDKExceptionMessage *string `json:"FinalSdkExceptionMessage,omitempty"`

	DestinationIP    *string `json:"DestinationIp,omitempty"`
	ConnectionReused *int    `json:"ConnectionReused,omitempty"`

	AcquireConnectionLatency *int `json:"AcquireConnectionLatency,omitempty"`
	ConnectLatency           *int `json:"ConnectLatency,omitempty"`
	RequestLatency           *int `json:"RequestLatency,omitempty"`
	DNSLatency               *int `json:"DnsLatency,omitempty"`
	TCPLatency               *int `json:"TcpLatency,omitempty"`
	SSLLatency               *int `json:"SslLatency,omitempty"`

	MaxRetriesExceeded *int `json:"MaxRetriesExceeded,omitempty"`

	// custom
	StandardUnit *string     `json:"StandardUnit,omitempty"`
	Dimensions   []Dimension `json:"Dimensions,omitempty"`
	RawValue     *float64    `json:"RawValue,omitempty"`
}

type Dimension struct {
	Name  *string `json:"Name,omitempty"`
	Value *string `json:"Value,omitempty"`
}
