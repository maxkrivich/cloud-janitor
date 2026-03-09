package domain

// Account represents an AWS account configuration.
type Account struct {
	// ID is the 12-digit AWS account ID.
	ID string

	// Name is a human-readable name for the account.
	Name string

	// RoleARN is the IAM role ARN to assume for accessing this account.
	RoleARN string
}
