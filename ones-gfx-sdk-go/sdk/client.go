package sdk

import (
	"github.com/aviznetworks/ones-gfx-sdk/ones-gfx-sdk-go/ones_gfx"
	"github.com/aviznetworks/ones-gfx-sdk/ones-gfx-sdk-go/ones_gfx/resources"
)

// Client is the main entry point for the SDK. It holds the transport and
// exposes resource handles for fabrics, tenants, operations, and peering.
type Client struct {
	transport  *ones_gfx.Transport
	Fabrics    *resources.FabricsResource
	Tenants    *resources.TenantsResource
	Operations *resources.OperationsResource
	Peering    *resources.PeeringResource
}

// NewClient constructs a Client with the given base URL and authentication.
//
// Example:
//
//	auth, err := ones_gfx.NewJWTAuth(accessToken, refreshToken, refreshURL)
//	if err != nil {
//	    panic(err)
//	}
//	client := sdk.NewClient("https://10.4.5.76:8089", auth,
//	    ones_gfx.WithClientTimeout(20*time.Minute),
//	    ones_gfx.WithTLSVerify(false))
//	defer client.Close()
func NewClient(baseURL string, auth ones_gfx.AuthProvider, opts ...ones_gfx.ClientOption) *Client {
	transport := ones_gfx.NewTransportWithOptions(baseURL, auth, opts...)
	return &Client{
		transport:  transport,
		Fabrics:    resources.NewFabricsResource(transport),
		Tenants:    resources.NewTenantsResource(transport),
		Operations: resources.NewOperationsResource(transport),
		Peering:    resources.NewPeeringResource(transport),
	}
}

// GetTransport returns the underlying transport for advanced use cases
// such as making raw API calls not covered by resource methods.
func (c *Client) GetTransport() *ones_gfx.Transport {
	return c.transport
}

// Close releases HTTP resources. Safe to call multiple times.
func (c *Client) Close() error {
	return c.transport.Close()
}
