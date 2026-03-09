package domain

// CloudProvider identifies the cloud platform.
type CloudProvider string

const (
	// ProviderAWS represents Amazon Web Services.
	ProviderAWS CloudProvider = "aws"
	// ProviderGCP represents Google Cloud Platform.
	ProviderGCP CloudProvider = "gcp"
	// ProviderAzure represents Microsoft Azure.
	ProviderAzure CloudProvider = "azure"
)

// String returns the string representation of the cloud provider.
func (p CloudProvider) String() string {
	return string(p)
}
