package util

import (
	"fmt"
	"math/rand"
	"net"
	"pkg/model"
	"strings"
	"time"
)

func GenerateUser(s Size) model.User {
	r := rand.Int31()
	out := model.User{}

	out.ID = model.ID{ID: fmt.Sprintf("ID:%d", r)}

	if r%2 == 0 {
		v := fmt.Sprintf("Username:%d", r)
		out.Username = &v
	}
	out.Email = fmt.Sprintf("Email:%d", r)

	out.Age = int(r)
	out.ReputationScore = int64(r)
	out.LoginCount = uint(r)
	out.FailedLoginAttempts = 5

	out.Enabled = r%2 == 1
	out.EmailVerified = r%2 == 1
	out.AccountBalance = rand.Float64()
	out.CompletionRate = rand.Float32()

	if r%2 == 0 {
		v := fmt.Sprintf("DisplayName:%d", r)
		out.DisplayName = &v
	}
	if r%2 == 0 {
		v := fmt.Sprintf("PhoneNumber:%d", r)
		out.PhoneNumber = &v
	}
	if r%2 == 0 {
		t := time.Now()
		out.LastLoginAt = &t
	}

	t, err := time.Parse(
		"2006-01-02",
		fmt.Sprintf("%d-%d-%d", r%40+1980, r%12+1, r%28+1),
	)
	if err != nil {
		t = time.Now()
	}
	out.BirthDate = t

	out.ProfilePicture = GeneratePicture(16, 16)
	out.Roles = []string{"ROLE_USER", fmt.Sprintf("ROLE_USER_%d", r)}

	lim := int(rand.Int31n(16))
	if lim > 0 {
		out.LoginIPs = make([]string, 0, lim)
		for c := 0; c < lim; c++ {
			ip := net.IPv4(
				byte(rand.Int31n(254)+1),
				byte(rand.Int31n(255)),
				byte(rand.Int31n(255)),
				byte(rand.Int31n(255)),
			)
			out.LoginIPs = append(out.LoginIPs, ip.String())
		}
	}

	switch r % 4 {
	case 1:
		out.NotificationChannels = []string{"sms"}
	case 2:
		out.NotificationChannels = []string{"email"}
	case 3:
		out.NotificationChannels = []string{"sms", "email"}
	}

	out.Settings = map[string]string{"test": "test"}
	out.FeatureFlags = map[string]bool{"test": true}
	out.QuotaByRegion = map[string]int{"test": int(r)}

	out.PrimaryAddress = GenerateAddress()
	if r%2 == 0 {
		a := GenerateAddress()
		out.SecondaryAddress = &a
	}

	addrCount := int(r % 4)
	if addrCount > 0 {
		out.Addresses = make([]model.Address, addrCount)
		for i := range out.Addresses {
			out.Addresses[i] = GenerateAddress()
		}
	}

	out.Timestampable = model.Timestampable{CreatedAt: time.Now()}

	out.SetPassword(fmt.Sprintf("password:%d", r))

	target := sizeTargetBytes(s)
	if target > 0 {
		current := estimateUserValueLength(out)
		if current < target {
			need := target - current
			if _, exists := out.Settings["payload"]; !exists {
				need -= len("payload")
			}
			if need < 0 {
				need = 0
			}
			out.Settings["payload"] = out.Settings["payload"] + strings.Repeat("x", need)
		}
	}

	return out
}

func sizeTargetBytes(s Size) int {
	switch s {
	case Size1KB:
		return 1 * 1024
	case Size10KB:
		return 10 * 1024
	case Size100KB:
		return 100 * 1024
	case Size300KB:
		return 300 * 1024
	default:
		return 1 * 1024
	}
}

func estimateUserValueLength(u model.User) int {
	// password is unexported in model.User, but SetPassword stores a SHA-512 sum as a 64-byte string.
	total := 64

	total += len(u.ID.ID)
	total += lenStringPtr(u.Username)
	total += len(u.Email)

	total += len(fmt.Sprintf("%d", u.Age))
	total += len(fmt.Sprintf("%d", u.ReputationScore))
	total += len(fmt.Sprintf("%d", u.LoginCount))
	total += len(fmt.Sprintf("%d", u.FailedLoginAttempts))

	total += len(fmt.Sprintf("%t", u.Enabled))
	total += len(fmt.Sprintf("%t", u.EmailVerified))
	total += len(fmt.Sprintf("%f", u.AccountBalance))
	total += len(fmt.Sprintf("%f", u.CompletionRate))

	total += lenStringPtr(u.DisplayName)
	total += lenStringPtr(u.PhoneNumber)
	total += lenTimePtr(u.LastLoginAt)
	total += len(u.BirthDate.Format(time.RFC3339Nano))

	total += len(u.ProfilePicture)
	total += lenStringSlice(u.Roles)
	total += lenStringSlice(u.LoginIPs)
	total += lenStringSlice(u.NotificationChannels)

	total += lenStringStringMap(u.Settings)
	total += lenStringBoolMap(u.FeatureFlags)
	total += lenStringIntMap(u.QuotaByRegion)

	total += lenAddress(u.PrimaryAddress)
	total += lenAddressPtr(u.SecondaryAddress)
	total += lenAddressSlice(u.Addresses)

	total += len(u.Timestampable.CreatedAt.Format(time.RFC3339Nano))
	total += len(u.Timestampable.UpdatedAt.Format(time.RFC3339Nano))
	total += lenTimePtr(u.Timestampable.DeletedAt)

	return total
}

func lenStringPtr(v *string) int {
	if v == nil {
		return 0
	}

	return len(*v)
}

func lenTimePtr(v *time.Time) int {
	if v == nil {
		return 0
	}

	return len(v.Format(time.RFC3339Nano))
}

func lenStringSlice(v []string) int {
	total := 0
	for _, s := range v {
		total += len(s)
	}

	return total
}

func lenStringStringMap(v map[string]string) int {
	total := 0
	for k, s := range v {
		total += len(k) + len(s)
	}

	return total
}

func lenStringBoolMap(v map[string]bool) int {
	total := 0
	for k, b := range v {
		total += len(k) + len(fmt.Sprintf("%t", b))
	}

	return total
}

func lenStringIntMap(v map[string]int) int {
	total := 0
	for k, n := range v {
		total += len(k) + len(fmt.Sprintf("%d", n))
	}

	return total
}

func lenAddress(a model.Address) int {
	return lenStringPtr(a.Name) +
		len(a.Country) +
		len(a.City) +
		len(a.County) +
		len(a.Street) +
		len(a.Number) +
		len(a.Zip) +
		len(a.Apartment)
}

func lenAddressPtr(a *model.Address) int {
	if a == nil {
		return 0
	}

	return lenAddress(*a)
}

func lenAddressSlice(v []model.Address) int {
	total := 0
	for _, a := range v {
		total += lenAddress(a)
	}

	return total
}

// generate random 16 by 16 picture PBM format
func GeneratePicture(w, h int) []byte {
	r := rand.Int31()
	picture := fmt.Sprintf("P1\n%d %d\n", w, h)

	for i := 0; i < w; i++ {
		for j := 0; j < h; j++ {
			if (r+int32(i)+int32(j))%2 == 1 {
				picture += "1 "
			} else {
				picture += "0 "
			}
		}
		picture += "\n"
	}

	return []byte(picture)
}

func GenerateAddress() model.Address {
	r := rand.Int31()

	return model.Address{
		Name: func() *string {
			if r%2 == 1 {
				return nil
			}

			s := fmt.Sprintf("Name:%d", r)

			return &s
		}(),
		Country:   fmt.Sprintf("Country:%d", r),
		City:      fmt.Sprintf("City:%d", r),
		County:    fmt.Sprintf("County:%d", r),
		Street:    fmt.Sprintf("Street:%d", r),
		Number:    fmt.Sprintf("Number:%d", r),
		Zip:       fmt.Sprintf("Zip:%d", r),
		Apartment: fmt.Sprintf("Apartment:%d", r),
	}
}
