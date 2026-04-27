package main

import "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
import "strconv"
import "pkg/util"
import "github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
import "time"
import "pkg/model"

func (ss *User) UnmarshalDynamoDBAttributeValue(in types.AttributeValue) error {
	m := in.(*types.AttributeValueMemberM).Value

	if raw, ok := m["id"]; ok && raw != nil {
		if mv, ok := raw.(*types.AttributeValueMemberS); ok {
			ss.ID.ID = mv.Value
		}
	}
	if raw, ok := m["username"]; ok && raw != nil {
		if mv, ok := raw.(*types.AttributeValueMemberS); ok {
			ss.Username = util.Pointer(mv.Value)
		}
	}
	if raw, ok := m["email"]; ok && raw != nil {
		if mv, ok := raw.(*types.AttributeValueMemberS); ok {
			ss.Email = mv.Value
		}
	}
	if raw, ok := m["password"]; ok && raw != nil {
		if mv, ok := raw.(*types.AttributeValueMemberS); ok {
			ss.password = mv.Value
		}
	}
	if raw, ok := m["age"]; ok && raw != nil {
		if mv, ok := raw.(*types.AttributeValueMemberN); ok {
			ss.Age = func() int { v, _ := strconv.ParseInt(mv.Value, 10, strconv.IntSize); return int(v) }()
		}
	}
	if raw, ok := m["reputation_score"]; ok && raw != nil {
		if mv, ok := raw.(*types.AttributeValueMemberN); ok {
			ss.ReputationScore = func() int64 { v, _ := strconv.ParseInt(mv.Value, 10, 64); return v }()
		}
	}
	if raw, ok := m["login_count"]; ok && raw != nil {
		if mv, ok := raw.(*types.AttributeValueMemberN); ok {
			ss.LoginCount = func() uint { v, _ := strconv.ParseUint(mv.Value, 10, strconv.IntSize); return uint(v) }()
		}
	}
	if raw, ok := m["failed_login_attempts"]; ok && raw != nil {
		if mv, ok := raw.(*types.AttributeValueMemberN); ok {
			ss.FailedLoginAttempts = func() uint8 { v, _ := strconv.ParseUint(mv.Value, 10, 8); return uint8(v) }()
		}
	}
	if raw, ok := m["enabled"]; ok && raw != nil {
		if mv, ok := raw.(*types.AttributeValueMemberBOOL); ok {
			ss.Enabled = mv.Value
		}
	}
	if raw, ok := m["email_verified"]; ok && raw != nil {
		if mv, ok := raw.(*types.AttributeValueMemberBOOL); ok {
			ss.EmailVerified = mv.Value
		}
	}
	if raw, ok := m["account_balance"]; ok && raw != nil {
		if mv, ok := raw.(*types.AttributeValueMemberN); ok {
			ss.AccountBalance = func() float64 { v, _ := strconv.ParseFloat(mv.Value, 64); return v }()
		}
	}
	if raw, ok := m["completion_rate"]; ok && raw != nil {
		if mv, ok := raw.(*types.AttributeValueMemberN); ok {
			ss.CompletionRate = func() float32 { v, _ := strconv.ParseFloat(mv.Value, 32); return float32(v) }()
		}
	}
	if raw, ok := m["display_name"]; ok && raw != nil {
		if mv, ok := raw.(*types.AttributeValueMemberS); ok {
			ss.DisplayName = util.Pointer(mv.Value)
		}
	}
	if raw, ok := m["phone_number"]; ok && raw != nil {
		if mv, ok := raw.(*types.AttributeValueMemberS); ok {
			ss.PhoneNumber = util.Pointer(mv.Value)
		}
	}
	if raw, ok := m["last_login_at"]; ok && raw != nil {
		ss.LastLoginAt = util.Pointer(func() time.Time { var out time.Time; _ = attributevalue.Unmarshal(raw, &out); return out }())
	}
	if raw, ok := m["birth_date"]; ok && raw != nil {
		ss.BirthDate = func() time.Time { var out time.Time; _ = attributevalue.Unmarshal(raw, &out); return out }()
	}
	if raw, ok := m["profile_picture"]; ok && raw != nil {
		if mv, ok := raw.(*types.AttributeValueMemberB); ok {
			ss.ProfilePicture = mv.Value
		}
	}
	if raw, ok := m["roles"]; ok && raw != nil {
		ss.Roles = func() []string { av, ok := raw.(*types.AttributeValueMemberL); if !ok { return nil }; out := make([]string, len(av.Value)); for i, item := range av.Value { if mv, ok := item.(*types.AttributeValueMemberS); ok { out[i] = mv.Value } }; return out }()
	}
	if raw, ok := m["login_ips"]; ok && raw != nil {
		ss.LoginIPs = func() []string { av, ok := raw.(*types.AttributeValueMemberL); if !ok { return nil }; out := make([]string, len(av.Value)); for i, item := range av.Value { if mv, ok := item.(*types.AttributeValueMemberS); ok { out[i] = mv.Value } }; return out }()
	}
	if raw, ok := m["notification_channels"]; ok && raw != nil {
		ss.NotificationChannels = func() []string { av, ok := raw.(*types.AttributeValueMemberL); if !ok { return nil }; out := make([]string, len(av.Value)); for i, item := range av.Value { if mv, ok := item.(*types.AttributeValueMemberS); ok { out[i] = mv.Value } }; return out }()
	}
	if raw, ok := m["settings"]; ok && raw != nil {
		ss.Settings = func() map[string]string { av, ok := raw.(*types.AttributeValueMemberM); if !ok { return nil }; out := make(map[string]string, len(av.Value)); for k, item := range av.Value { if mv, ok := item.(*types.AttributeValueMemberS); ok { out[k] = mv.Value } }; return out }()
	}
	if raw, ok := m["feature_flags"]; ok && raw != nil {
		ss.FeatureFlags = func() map[string]bool { av, ok := raw.(*types.AttributeValueMemberM); if !ok { return nil }; out := make(map[string]bool, len(av.Value)); for k, item := range av.Value { if mv, ok := item.(*types.AttributeValueMemberBOOL); ok { out[k] = mv.Value } }; return out }()
	}
	if raw, ok := m["quota_by_region"]; ok && raw != nil {
		ss.QuotaByRegion = func() map[string]int { av, ok := raw.(*types.AttributeValueMemberM); if !ok { return nil }; out := make(map[string]int, len(av.Value)); for k, item := range av.Value { if mv, ok := item.(*types.AttributeValueMemberN); ok { out[k] = func() int { v, _ := strconv.ParseInt(mv.Value, 10, strconv.IntSize); return int(v) }() } }; return out }()
	}
	if raw, ok := m["primary_address"]; ok && raw != nil {
		ss.PrimaryAddress = func() model.Address { var out model.Address; _ = attributevalue.Unmarshal(raw, &out); return out }()
	}
	if raw, ok := m["secondary_address"]; ok && raw != nil {
		ss.SecondaryAddress = util.Pointer(func() model.Address { var out model.Address; _ = attributevalue.Unmarshal(raw, &out); return out }())
	}
	if raw, ok := m["addresses"]; ok && raw != nil {
		ss.Addresses = func() []model.Address { var out []model.Address; _ = attributevalue.Unmarshal(raw, &out); return out }()
	}
	if raw, ok := m["created_at"]; ok && raw != nil {
		ss.Timestampable.CreatedAt = func() time.Time { var out time.Time; _ = attributevalue.Unmarshal(raw, &out); return out }()
	}
	if raw, ok := m["updated_at"]; ok && raw != nil {
		ss.Timestampable.UpdatedAt = func() time.Time { var out time.Time; _ = attributevalue.Unmarshal(raw, &out); return out }()
	}
	if raw, ok := m["deleted_at"]; ok && raw != nil {
		ss.Timestampable.DeletedAt = util.Pointer(func() time.Time { var out time.Time; _ = attributevalue.Unmarshal(raw, &out); return out }())
	}

	return nil
}

func (ss *User) MarshalDynamoDBAttributeValue() (types.AttributeValue, error) {
	out := make(map[string]types.AttributeValue, 29)

	out["id"] = &types.AttributeValueMemberS{
		Value: ss.ID.ID,
	}
	out["username"] = &types.AttributeValueMemberS{
		Value: util.Unwrap(ss.Username),
	}
	out["email"] = &types.AttributeValueMemberS{
		Value: ss.Email,
	}
	out["password"] = &types.AttributeValueMemberS{
		Value: ss.password,
	}
	out["age"] = &types.AttributeValueMemberN{
		Value: strconv.FormatInt(int64(ss.Age), 10),
	}
	out["reputation_score"] = &types.AttributeValueMemberN{
		Value: strconv.FormatInt(ss.ReputationScore, 10),
	}
	out["login_count"] = &types.AttributeValueMemberN{
		Value: strconv.FormatUint(uint64(ss.LoginCount), 10),
	}
	out["failed_login_attempts"] = &types.AttributeValueMemberN{
		Value: strconv.FormatUint(uint64(ss.FailedLoginAttempts), 10),
	}
	out["enabled"] = &types.AttributeValueMemberBOOL{
		Value: ss.Enabled,
	}
	out["email_verified"] = &types.AttributeValueMemberBOOL{
		Value: ss.EmailVerified,
	}
	out["account_balance"] = &types.AttributeValueMemberN{
		Value: strconv.FormatFloat(ss.AccountBalance, 'f', -1, 64),
	}
	out["completion_rate"] = &types.AttributeValueMemberN{
		Value: strconv.FormatFloat(float64(ss.CompletionRate), 'f', -1, 32),
	}
	out["display_name"] = &types.AttributeValueMemberS{
		Value: util.Unwrap(ss.DisplayName),
	}
	out["phone_number"] = &types.AttributeValueMemberS{
		Value: util.Unwrap(ss.PhoneNumber),
	}
	if av, err := attributevalue.Marshal(util.Unwrap(ss.LastLoginAt)); err != nil {
		return nil, err
	} else {
		out["last_login_at"] = av
	}
	if av, err := attributevalue.Marshal(ss.BirthDate); err != nil {
		return nil, err
	} else {
		out["birth_date"] = av
	}
	out["profile_picture"] = &types.AttributeValueMemberB{
		Value: ss.ProfilePicture,
	}
	out["roles"] = &types.AttributeValueMemberL{Value: func() []types.AttributeValue { if ss.Roles == nil { return nil }; out := make([]types.AttributeValue, 0, len(ss.Roles)); for _, v := range ss.Roles { out = append(out, &types.AttributeValueMemberS{Value: v}) }; return out }()}
	out["login_ips"] = &types.AttributeValueMemberL{Value: func() []types.AttributeValue { if ss.LoginIPs == nil { return nil }; out := make([]types.AttributeValue, 0, len(ss.LoginIPs)); for _, v := range ss.LoginIPs { out = append(out, &types.AttributeValueMemberS{Value: v}) }; return out }()}
	out["notification_channels"] = &types.AttributeValueMemberL{Value: func() []types.AttributeValue { if ss.NotificationChannels == nil { return nil }; out := make([]types.AttributeValue, 0, len(ss.NotificationChannels)); for _, v := range ss.NotificationChannels { out = append(out, &types.AttributeValueMemberS{Value: v}) }; return out }()}
	out["settings"] = &types.AttributeValueMemberM{Value: func() map[string]types.AttributeValue { if ss.Settings == nil { return nil }; out := make(map[string]types.AttributeValue, len(ss.Settings)); for k, v := range ss.Settings { out[k] = &types.AttributeValueMemberS{Value: v} }; return out }()}
	out["feature_flags"] = &types.AttributeValueMemberM{Value: func() map[string]types.AttributeValue { if ss.FeatureFlags == nil { return nil }; out := make(map[string]types.AttributeValue, len(ss.FeatureFlags)); for k, v := range ss.FeatureFlags { out[k] = &types.AttributeValueMemberBOOL{Value: v} }; return out }()}
	out["quota_by_region"] = &types.AttributeValueMemberM{Value: func() map[string]types.AttributeValue { if ss.QuotaByRegion == nil { return nil }; out := make(map[string]types.AttributeValue, len(ss.QuotaByRegion)); for k, v := range ss.QuotaByRegion { out[k] = &types.AttributeValueMemberN{Value: strconv.FormatInt(int64(v), 10)} }; return out }()}
	if av, err := attributevalue.Marshal(ss.PrimaryAddress); err != nil {
		return nil, err
	} else {
		out["primary_address"] = av
	}
	if av, err := attributevalue.Marshal(util.Unwrap(ss.SecondaryAddress)); err != nil {
		return nil, err
	} else {
		out["secondary_address"] = av
	}
	if av, err := attributevalue.Marshal(ss.Addresses); err != nil {
		return nil, err
	} else {
		out["addresses"] = av
	}
	if av, err := attributevalue.Marshal(ss.Timestampable.CreatedAt); err != nil {
		return nil, err
	} else {
		out["created_at"] = av
	}
	if av, err := attributevalue.Marshal(ss.Timestampable.UpdatedAt); err != nil {
		return nil, err
	} else {
		out["updated_at"] = av
	}
	if av, err := attributevalue.Marshal(util.Unwrap(ss.Timestampable.DeletedAt)); err != nil {
		return nil, err
	} else {
		out["deleted_at"] = av
	}

	return &types.AttributeValueMemberM{Value: out}, nil
}
