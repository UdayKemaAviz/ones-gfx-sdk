# ONES GFX SDK

Multi-language SDK for **AVIZ ONES Spectrum-X** tenant management API (v4.2).

Provides programmatic access to:
- Tenant lifecycle management (create, read, update, delete)
- GPU server allocation and deallocation
- Fabric discovery and configuration
- VPC peering setup
- Asynchronous operation tracking (sync / async-poll / async-webhook)

---

## Choose Your Language

### [📘 Python SDK](./ones-gfx-sdk-python/)
**Status:** Production-ready (v0.1.0)

- **Requirements:** Python 3.9+
- **Installation:** `pip install ./ones-gfx-sdk-python`
- **Use Case:** Data science workflows, Jupyter notebooks, automation scripts, rapid prototyping
- **Features:** Full API coverage, JWT auto-refresh, timeout override, async modes

**Quick Start:**
```python
from ones_gfx import ONESClient, JWTAuth

auth = JWTAuth(access_token, refresh_token, refresh_url)
client = ONESClient(base_url, auth)
tenant = client.tenants.create(fabric, "tenant1", "desc", max_gpus=8)
```

[📖 Python Documentation →](./ones-gfx-sdk-python/README.md)

---

### [📗 Go SDK](./ones-gfx-sdk-go/)
**Status:** Core library production-ready (v1.0.0), CLI in development

- **Requirements:** Go 1.19+
- **Installation:** `go get github.com/aviznetworks/ones-gfx-sdk/ones-gfx-sdk-go`
- **Use Case:** Kubernetes operators, controllers, high-performance integrations, compiled binaries
- **Features:** Type-safe, zero dependencies, compile-time error checking, single binary deployment

**Quick Start:**
```go
import (
    "github.com/aviznetworks/ones-gfx-sdk/ones-gfx-sdk-go/ones"
    "github.com/aviznetworks/ones-gfx-sdk/ones-gfx-sdk-go/sdk"
)

auth, err := ones.NewJWTAuth(accessToken, refreshToken, refreshURL)
if err != nil {
    panic(err)
}
client := sdk.NewClient(baseURL, auth)
tenant, err := client.Tenants.Create(ctx, fabricName, req)
```

[📖 Go Documentation →](./ones-gfx-sdk-go/README.md)

---

## Feature Comparison

| Feature | Python | Go | Notes |
|---------|--------|-----|-------|
| **Tenant CRUD** | ✅ | ✅ | Full parity |
| **GPU allocation/deallocation** | ✅ | ✅ | Timeout override supported |
| **Async modes** | ✅ | ✅ | sync / async-poll / async-webhook |
| **JWT auto-refresh** | ✅ | ✅ | Proactive + reactive |
| **Timeout override** | ✅ | ✅ | Per-call override for long operations |
| **CLI with JSON output** | ✅ | 🚧 | Go: v1.1 (in development) |
| **Config file support** | ❌ | 🚧 | Go: v1.1 (in development) |
| **Library import** | ✅ | ✅ | Use as dependency |
| **Type safety** | Runtime | Compile-time | Go catches errors before deployment |
| **Binary distribution** | ❌ | ✅ | Go compiles to single binary |
| **Memory footprint** | ~30MB | ~5MB | Go binary overhead |
| **Startup time** | ~100ms | ~1ms | Interpreted vs compiled |
| **Dependencies** | 1 (`requests`) | 0 | Go uses only stdlib |

---

## Installation Quick Reference

### Python
```bash
# From PyPI (when published)
pip install ones-gfx-sdk

# From source
cd ones-gfx-sdk/ones-gfx-sdk-python
pip install -e .

# CLI usage
python examples/usage_examples.py --mode sync --action lifecycle
```

### Go
```bash
# As library
go get github.com/aviznetworks/ones-gfx-sdk/ones-gfx-sdk-go

# From source
cd ones-gfx-sdk/ones-gfx-sdk-go
make build-lib

# CLI usage (coming in v1.1)
# ones-gfx-sdk-mod lifecycle --mode sync --fabric sdk
```

---

## Controller Integration Examples

### Python Controller (Subprocess)
```python
import subprocess
import json

result = subprocess.run([
    "python", "examples/usage_examples.py",
    "--action", "create",
    "--tenant-name", "controller-tenant",
    "--fabric", "sdk",
    "--output", "json"
], capture_output=True, text=True)

tenant = json.loads(result.stdout)
print(f"Created tenant ID: {tenant['tenant']['id']}")
```

### Go Controller (Library Import)
```go
import "github.com/aviznetworks/ones-gfx-sdk/ones-gfx-sdk-go/ones"

client := sdk.NewClient(baseURL, auth)
tenant, err := client.Tenants.Create(ctx, "sdk", ones.CreateTenantRequest{
    Name: "controller-tenant", MaxGPUsAllowed: 8,
})
```

---

## Repository Structure

```
ones-gfx-sdk/
├── README.md                    # This file (language picker)
├── LICENSE                      # Apache 2.0
├── ones-gfx-sdk-python/         # Python implementation
│   ├── ones_gfx/                # Core library package
│   ├── examples/                # CLI with 8 commands
│   ├── pyproject.toml
│   └── README.md
└── ones-gfx-sdk-go/             # Go implementation
    ├── ones/                    # Core library (importable)
    ├── cmd/                     # CLI binary (v1.1)
    ├── go.mod
    └── README.md
```

---

## Common Workflows

### Full Tenant Lifecycle (Python)
```bash
cd ones-gfx-sdk-python
python examples/usage_examples.py --mode sync --action lifecycle --fabric sdk
```

### Full Tenant Lifecycle (Go — Library)
```go
// See ones-gfx-sdk-go/README.md for complete example
tenant, _ := client.Tenants.Create(ctx, fabricName, req)
_ = client.Tenants.AllocateGPUs(ctx, fabricName, name, servers)
_ = client.Tenants.DeallocateGPUs(ctx, fabricName, name, servers)
_ = client.Tenants.Delete(ctx, fabricName, name)
```

---

## API Reference

Both SDKs wrap the same ONES Spectrum-X API. For raw curl examples, see:

📄 [api_reference.txt](./ones-gfx-sdk-python/api_reference.txt) — Language-agnostic API examples

**Endpoints covered:**
- `POST /fabrics/{fabricName}/tenants` — Create tenant
- `GET /fabrics/{fabricName}/tenants` — List tenants
- `GET /fabrics/{fabricName}/tenants/{name}` — Get tenant
- `DELETE /fabrics/{fabricName}/tenants/{name}` — Delete tenant
- `PATCH /fabrics/{fabricName}/tenants/{name}` — Allocate/deallocate GPUs
- `GET /fabrics` — List fabrics
- `GET /operations/{id}` — Poll async operation
- `POST /fabrics/{fabricName}/vpcpeering` — VPC peering

---

## Version Compatibility

| ONES API Version | Python SDK | Go SDK |
|------------------|------------|---------|
| 4.2.1 | ✅ v0.1.0 | ✅ v1.0.0 |

Both SDKs target **ONES 4.2.1**. 

---

## Contributing

See language-specific contribution guidelines:
- [Python Contributing Guide](./ones-gfx-sdk-python/CONTRIBUTING.md) *(coming soon)*
- [Go Contributing Guide](./ones-gfx-sdk-go/CONTRIBUTING.md) *(coming soon)*

---

## Support

- **Documentation:** See language-specific READMEs
- **Issues:** [GitHub Issues](https://github.com/aviznetworks/ones-gfx-sdk/issues)
- **Security:** Report vulnerabilities to security@aviznetworks.com

---

## License

Apache 2.0 — See [LICENSE](./LICENSE) for details.

---

**Developed by AVIZ Networks**  
For questions or support, visit https://aviznetworks.com
