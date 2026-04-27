package model

import (
	"crypto/sha512"
	"crypto/subtle"
	"time"
)

type Timestampable struct {
	CreatedAt time.Time  `dynamodbav:"created_at"`
	UpdatedAt time.Time  `dynamodbav:"updated_at"`
	DeletedAt *time.Time `dynamodbav:"deleted_at"`
}

type Address struct {
	Name      *string `dynamodbav:"name,omitempty"`
	Country   string  `dynamodbav:"country,omitempty"`
	City      string  `dynamodbav:"city,omitempty"`
	County    string  `dynamodbav:"county,omitempty"`
	Street    string  `dynamodbav:"street,omitempty"`
	Number    string  `dynamodbav:"number,omitempty"`
	Zip       string  `dynamodbav:"zip,omitempty"`
	Apartment string  `dynamodbav:"apartment,omitempty"`
}

type User struct {
	ID

	Username *string `dynamodbav:"username"`
	Email    string  `dynamodbav:"email"`
	password string  `dynamodbav:"password"`

	Age                 int   `dynamodbav:"age"`
	ReputationScore     int64 `dynamodbav:"reputation_score"`
	LoginCount          uint  `dynamodbav:"login_count"`
	FailedLoginAttempts uint8 `dynamodbav:"failed_login_attempts"`

	Enabled        bool    `dynamodbav:"enabled"`
	EmailVerified  bool    `dynamodbav:"email_verified"`
	AccountBalance float64 `dynamodbav:"account_balance"`
	CompletionRate float32 `dynamodbav:"completion_rate"`

	DisplayName *string    `dynamodbav:"display_name"`
	PhoneNumber *string    `dynamodbav:"phone_number"`
	LastLoginAt *time.Time `dynamodbav:"last_login_at"`
	BirthDate   time.Time  `dynamodbav:"birth_date"`

	ProfilePicture       []byte   `dynamodbav:"profile_picture"`
	Roles                []string `dynamodbav:"roles"`
	LoginIPs             []string `dynamodbav:"login_ips"`
	NotificationChannels []string `dynamodbav:"notification_channels"`

	Settings      map[string]string `dynamodbav:"settings"`
	FeatureFlags  map[string]bool   `dynamodbav:"feature_flags"`
	QuotaByRegion map[string]int    `dynamodbav:"quota_by_region"`

	PrimaryAddress   Address   `dynamodbav:"primary_address"`
	SecondaryAddress *Address  `dynamodbav:"secondary_address"`
	Addresses        []Address `dynamodbav:"addresses"`

	Timestampable
}

func (u *User) SetPassword(p string) {
	sum := sha512.Sum512([]byte(p))
	u.password = string(sum[:])
}

func (u *User) PasswordMatches(p string) bool {
	sum := sha512.Sum512([]byte(p))

	return subtle.ConstantTimeCompare([]byte(u.password), sum[:]) == 1
}

type ID struct {
	ID string `dynamodbav:"id"`
}

func (i ID) String() string {
	return i.ID
}
