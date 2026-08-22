package domain

type PaymentFilter struct {
	OrganizationID string
	Status         string
	Provider       string
	MerchantID     string
	ProviderRef    string
	Search         string
	Page           int
	PageSize       int
}
