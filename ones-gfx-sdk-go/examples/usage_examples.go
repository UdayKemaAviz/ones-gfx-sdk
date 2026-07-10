/*
End-to-end usage examples for the ONES Spectrum-X Go SDK.

This file is intended to be read top-to-bottom by partners as a tour of
the SDK's surface. It covers:

    1. Constructing the client with JWT auth.
    2. Reading fabrics and tenants.
    3. Running a full tenant lifecycle in a single mode (sync/async-poll/async-webhook).
    4. Error handling.

How to run
----------
1. Edit the CONFIG section below with your ONES base URL, JWT tokens,
   and a fabric name that exists in your environment.

2. Choose which mode to run by passing --mode (sync, async-poll,
   async-webhook). The default is sync.

3. Run the program:

       go run examples/usage_examples.go                    # from module root
       go run examples/usage_examples.go --mode async-poll  # async polling
       go run examples/usage_examples.go --action read-only # just list fabrics/tenants
*/
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aviznetworks/ones-gfx-sdk/ones-gfx-sdk-go/ones_gfx"
	"github.com/aviznetworks/ones-gfx-sdk/ones-gfx-sdk-go/ones_gfx/resources"
	"github.com/aviznetworks/ones-gfx-sdk/ones-gfx-sdk-go/sdk"
)

// ---------------------------------------------------------------------------
// CONFIG — loaded from .env file or environment variables.
// See ../.env.example for available configuration options.
// ---------------------------------------------------------------------------

var (
	baseURL    string
	refreshURL string
	accessToken  string
	refreshToken string
	loginUsername string
	loginPassword string
	fabricName string
	webhookURL string
	verifyTLS bool
	defaultTimeoutS int
)

// loadEnv loads configuration from .env file or environment variables.
func loadEnv() error {
	// Try to load .env file from multiple locations
	possiblePaths := []string{
		".env",           // current directory
		"examples/.env",  // examples subdirectory (if running from parent)
		"../.env",        // parent directory
		"../../.env",     // grandparent directory (repo root)
	}

	for _, envFile := range possiblePaths {
		if info, err := os.Stat(envFile); err == nil && !info.IsDir() {
			if err := loadEnvFile(envFile); err != nil {
				log.Printf("Warning: could not load %s: %v (trying next path)\n", envFile, err)
				continue
			}
			log.Printf("Loaded configuration from %s\n", envFile)
			break
		}
	}

	// Load from environment variables (these override .env file values)
	baseURL = getEnv("BASE_URL", "")
	refreshURL = getEnv("REFRESH_URL", "")
	accessToken = getEnv("ACCESS_TOKEN", "")
	refreshToken = getEnv("REFRESH_TOKEN", "")
	loginUsername = getEnv("LOGIN_USERNAME", "")
	loginPassword = getEnv("LOGIN_PASSWORD", "")
	fabricName = getEnv("FABRIC_NAME", "")
	webhookURL = getEnv("WEBHOOK_URL", "http://your_webhook_endpoint:5000/test/webhook-receiver")
	verifyTLS = getBoolEnv("VERIFY_TLS", false)
	defaultTimeoutS = getIntEnv("DEFAULT_TIMEOUT_S", 1200)

	// Validate required fields
	if baseURL == "" {
		return fmt.Errorf("BASE_URL environment variable not set (see ../../.env.example)")
	}
	if refreshURL == "" {
		return fmt.Errorf("REFRESH_URL environment variable not set (see ../../.env.example)")
	}
	if accessToken == "" {
		return fmt.Errorf("ACCESS_TOKEN environment variable not set (see ../../.env.example)")
	}
	if refreshToken == "" {
		return fmt.Errorf("REFRESH_TOKEN environment variable not set (see ../../.env.example)")
	}
	if loginUsername == "" {
		return fmt.Errorf("LOGIN_USERNAME environment variable not set (see ../../.env.example)")
	}
	if loginPassword == "" {
		return fmt.Errorf("LOGIN_PASSWORD environment variable not set (see ../../.env.example)")
	}
	if fabricName == "" {
		return fmt.Errorf("FABRIC_NAME environment variable not set (see ../../.env.example)")
	}

	return nil
}

// loadEnvFile parses a .env file and sets environment variables.
func loadEnvFile(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			// Only set if not already in environment
			if os.Getenv(key) == "" {
				os.Setenv(key, value)
			}
		}
	}
	return scanner.Err()
}

// getEnv retrieves an environment variable with a default value.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getIntEnv retrieves an environment variable as an integer.
func getIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
		log.Printf("Warning: invalid integer value for %s: %s\n", key, value)
	}
	return defaultValue
}

// getBoolEnv retrieves an environment variable as a boolean.
func getBoolEnv(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return strings.ToLower(value) == "true" || value == "1"
	}
	return defaultValue
}

// A couple of sample server hostnames you expect to be available in the
// fabric. The example will try to allocate then deallocate these.
var sampleServers = []string{"hgx-su00-h00"}

// ---------------------------------------------------------------------------
// Auth callback (optional) — persist rotated tokens so they survive a
// process restart. Replace the print with whatever your secret store
// requires.
// ---------------------------------------------------------------------------

func onTokenRefresh(access, refresh string, expiresIn int) {
	fmt.Printf("[auth] tokens rotated; new access expires in %ds\n", expiresIn)
	// Example:
	//   keyring.Set("ones_gfx", "access_token", access)
	//   keyring.Set("ones_gfx", "refresh_token", refresh)
}

func buildClient() *sdk.Client {
	auth, err := ones_gfx.NewJWTAuth(
		accessToken,
		refreshToken,
		refreshURL,
		ones_gfx.WithTokenRefreshCallback(onTokenRefresh),
		ones_gfx.WithAuthTLSVerify(verifyTLS),
	)
	if err != nil {
		log.Fatalf("failed to create auth: %v", err)
	}
	return sdk.NewClient(
		baseURL,
		auth,
		ones_gfx.WithTLSVerify(verifyTLS),
		ones_gfx.WithClientTimeout(time.Duration(defaultTimeoutS)*time.Second),
	)
}

// ---------------------------------------------------------------------------
// Scenario 1: list fabrics and tenants.
// ---------------------------------------------------------------------------

func scenarioReadOnly(client *sdk.Client) {
	fmt.Println("\n--- Scenario: read-only ---")
	ctx := context.Background()

	fabrics, err := client.Fabrics.List(ctx)
	if err != nil {
		fmt.Printf("Error listing fabrics: %v\n", err)
		return
	}
	fmt.Printf("Found %d fabric(s):\n", len(fabrics))
	for _, fab := range fabrics {
		fmt.Printf("  - %s (SUs=%d/%d, default storage=%s)\n",
			fab.FabricName, fab.NumOfSUs, fab.MaxNumOfSUs, fab.DefaultStorageName)
	}

	tenants, err := client.Tenants.List(ctx, fabricName)
	if err != nil {
		fmt.Printf("Error listing tenants: %v\n", err)
		return
	}
	fmt.Printf("\nTenants in %s: %d\n", fabricName, len(tenants))
	for _, t := range tenants {
		gpuSummary := fmt.Sprintf("%d/%d", t.GPUsAllocated, t.MaxGPUsAllowed)
		if t.IsUnlimitedGPUs() {
			gpuSummary = "unlimited"
		}
		fmt.Printf("  - %s | gpus=%s | status=%s\n", t.Name, gpuSummary, t.ConfigStatus)
	}

	available, err := client.Tenants.AvailableServers(ctx, fabricName)
	if err != nil {
		fmt.Printf("Error listing available servers: %v\n", err)
		return
	}
	fmt.Printf("\nAvailable servers in %s: %v\n", fabricName, available)
}

func scenarioLogin(username, password string) {
	fmt.Println("\n--- Scenario: login ---")
	payload := map[string]string{
		"username": username,
		"password": password,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("Error marshaling login payload: %v\n", err)
		return
	}

	loginURL := strings.TrimRight(baseURL, "/") + "/login"
	httpClient := &http.Client{
		Timeout: time.Duration(defaultTimeoutS) * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: !verifyTLS,
			},
		},
	}
	resp, err := httpClient.Post(loginURL, "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Printf("Login request failed: %v\n", err)
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading login response: %v\n", err)
		return
	}

	var result interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		fmt.Printf("Error parsing login response: %v\n", err)
		return
	}
	fmt.Println(result)
}

// ---------------------------------------------------------------------------
// Scenario 2: tenant lifecycle in a single mode.
// ---------------------------------------------------------------------------

func pollOperation(client *sdk.Client, operationID, label string, maxAttempts, pollIntervalS int) bool {
	fmt.Printf("  -> polling %s operation: %s\n", label, operationID)
	ctx := context.Background()
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		current, err := client.Operations.Get(ctx, operationID)
		if err != nil {
			fmt.Printf("\n  [poll %d] error: %v\n", attempt, err)
			return false
		}
		if current.Status == ones_gfx.OperationStatusRunning || current.Status == ones_gfx.OperationStatusPending {
			fmt.Print(".")
		} else {
			fmt.Printf("\n  [poll %d] status=%s\n", attempt, current.Status)
		}
		if current.IsDone() {
			fmt.Println()
			if current.IsSuccess() {
				return true
			}
			fmt.Printf("  -> FAILED: %s\n", current.ErrorMessage)
			return false
		}
		time.Sleep(time.Duration(pollIntervalS) * time.Second)
	}
	fmt.Println("  -> still not done after polling window; giving up")
	return false
}

func pollWebhookStatus(client *sdk.Client, operationID, label string, maxAttempts, pollIntervalS int) bool {
	path := fmt.Sprintf("operations/%s/webhook-status", operationID)
	fmt.Printf("  -> polling %s webhook delivery: %s\n", label, operationID)
	ctx := context.Background()
	_ = ctx // used conceptually; transport.Get doesn't take context yet
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Access the transport's Get method directly for webhook status
		result, err := client.GetTransport().Get(path, nil)
		if err != nil {
			fmt.Printf("\n  [poll %d] error: %v\n", attempt, err)
			return false
		}
		status := "PENDING"
		if obj, ok := result.(map[string]interface{}); ok {
			if ds, ok := obj["deliveryStatus"].(string); ok {
				status = ds
			}
		}
		if status == "SUCCESS" {
			fmt.Printf("\n  [poll %d] webhook deliveryStatus=%s\n", attempt, status)
			return true
		}
		if status == "FAILED" || status == "EXHAUSTED" {
			fmt.Printf("\n  [poll %d] webhook deliveryStatus=%s\n", attempt, status)
			fmt.Println("  -> FAILED: webhook delivery did not succeed")
			return false
		}
		fmt.Print(".")
		time.Sleep(time.Duration(pollIntervalS) * time.Second)
	}
	fmt.Println("\n  -> still not delivered after polling window; giving up")
	return false
}

func tenantNameForMode(mode string) string {
	switch mode {
	case "sync":
		return "sdk_sync"
	case "async-poll":
		return "sdk_async"
	default:
		return "sdk_hook"
	}
}

func modeLabel(mode string) string {
	switch mode {
	case "sync":
		return "sync"
	case "async-poll":
		return "async-poll"
	default:
		return "async-webhook"
	}
}

func defaultVPCName(tenantName string) string {
	return fmt.Sprintf("%s-%s-north-south", tenantName, fabricName)
}

func defaultPeerVPCName() string {
	return fmt.Sprintf("%s-Storage-VPC", fabricName)
}

// operationMode maps CLI mode string to ones_gfx.OperationMode.
func operationMode(mode string) ones_gfx.OperationMode {
	switch mode {
	case "sync":
		return ones_gfx.OperationModeSynchronous
	case "async-poll":
		return ones_gfx.OperationModeAsyncPoll
	default:
		return ones_gfx.OperationModeAsyncWebhook
	}
}

// isAsync returns true if the mode uses async operations.
func isAsync(mode string) bool {
	return mode == "async-poll" || mode == "async-webhook"
}

// isWebhook returns true if the mode uses webhook delivery.
func isWebhook(mode string) bool {
	return mode == "async-webhook"
}

// pollFn selects the appropriate poller based on mode.
func pollFn(mode string) func(*sdk.Client, string, string, int, int) bool {
	if isWebhook(mode) {
		return pollWebhookStatus
	}
	return pollOperation
}

func scenarioTenantLifecycle(client *sdk.Client, mode string) {
	label := modeLabel(mode)
	ctx := context.Background()

	fmt.Printf("\n--- Scenario: tenant lifecycle (%s) ---\n", label)

	tenantName := tenantNameForMode(mode)

	// --- Create ---
	fmt.Printf("Creating tenant %q (%s)...\n", tenantName, label)
	createReq := resources.CreateTenantRequest{
		Name:           tenantName,
		Description:    fmt.Sprintf("Created by SDK example (%s mode)", label),
		MaxGPUsAllowed: 8,
	}

	if isAsync(mode) {
		var opts []ones_gfx.CallOption
		if isWebhook(mode) {
			opts = append(opts, ones_gfx.WithWebhook(webhookURL, []string{"tenant.create"}))
		}
		op, err := client.Tenants.CreateAsync(ctx, fabricName, createReq, opts...)
		if err != nil {
			fmt.Printf("  -> create error: %v\n", err)
			return
		}
		fmt.Printf("  -> operation id: %s\n", op.ID)
		if isWebhook(mode) {
			fmt.Printf("  -> webhook registered: %v\n", op.WebhookRegistered)
		}
		if !pollFn(mode)(client, op.ID, "create", 360, 5) {
			return
		}
	} else {
		tenant, err := client.Tenants.Create(ctx, fabricName, createReq)
		if err != nil {
			fmt.Printf("  -> create error: %v\n", err)
			return
		}
		vlanID, vniID := 0, 0
		if tenant.VLANID != nil {
			vlanID = *tenant.VLANID
		}
		if tenant.VNIID != nil {
			vniID = *tenant.VNIID
		}
		fmt.Printf("  -> id=%d, vlan=%d, vni=%d\n", tenant.ID, vlanID, vniID)
	}

	// --- Allocate GPUs ---
	if len(sampleServers) > 0 {
		fmt.Printf("Allocating GPUs %v to %s (%s)...\n", sampleServers, tenantName, label)
		serverSpecs := resources.ServerSpecsFromNames(sampleServers)
		timeout500 := 500 * time.Second

		if isAsync(mode) {
			var opts []ones_gfx.CallOption
			opts = append(opts, ones_gfx.WithTimeout(timeout500))
			if isWebhook(mode) {
				opts = append(opts, ones_gfx.WithWebhook(webhookURL, []string{"tenant.allocate"}))
			}
			op, err := client.Tenants.AllocateGPUsAsync(ctx, fabricName, tenantName, serverSpecs, opts...)
			if err != nil {
				fmt.Printf("  -> allocate error: %v\n", err)
				return
			}
			fmt.Printf("  -> operation id: %s\n", op.ID)
			if !pollFn(mode)(client, op.ID, "allocate", 360, 5) {
				return
			}
		} else {
			err := client.Tenants.AllocateGPUs(ctx, fabricName, tenantName, serverSpecs, ones_gfx.WithTimeout(timeout500))
			if err != nil {
				fmt.Printf("  -> allocate error: %v\n", err)
				return
			}
			fmt.Println("  -> allocate done")
		}

		refreshed, err := client.Tenants.Get(ctx, fabricName, tenantName)
		if err != nil {
			fmt.Printf("  -> error fetching tenant: %v\n", err)
		} else {
			fmt.Printf("  Tenant now has servers: %v\n", refreshed.AllotedServers())
		}

		// --- VPC Peering (sync only): after allocate so tenant VPC is live with GPUs ---
		peeringName := fmt.Sprintf("%s-storage-route-leak", tenantName)
		vpcName := defaultVPCName(tenantName)
		peerVPCName := defaultPeerVPCName()
		fmt.Printf("Creating VPC peering %q between %q and %q (sync-only)...\n",
			peeringName, vpcName, peerVPCName)
		peeringResult, err := client.Peering.Create(ctx, fabricName, peeringName, vpcName, peerVPCName)
		if err != nil {
			fmt.Printf("  -> peering error: %v\n", err)
		} else {
			fmt.Printf("  -> response: %v\n", peeringResult)
		}

		// --- Deallocate GPUs ---
		fmt.Printf("Deallocating GPUs %v (%s)...\n", sampleServers, label)

		if isAsync(mode) {
			var opts []ones_gfx.CallOption
			opts = append(opts, ones_gfx.WithTimeout(timeout500))
			if isWebhook(mode) {
				opts = append(opts, ones_gfx.WithWebhook(webhookURL, []string{"tenant.deallocate"}))
			}
			op, err := client.Tenants.DeallocateGPUsAsync(ctx, fabricName, tenantName, serverSpecs, opts...)
			if err != nil {
				fmt.Printf("  -> deallocate error: %v\n", err)
				return
			}
			fmt.Printf("  -> operation id: %s\n", op.ID)
			if !pollFn(mode)(client, op.ID, "deallocate", 360, 5) {
				return
			}
		} else {
			err := client.Tenants.DeallocateGPUs(ctx, fabricName, tenantName, serverSpecs, ones_gfx.WithTimeout(timeout500))
			if err != nil {
				fmt.Printf("  -> deallocate error: %v\n", err)
				return
			}
			fmt.Println("  -> deallocate done")
		}
	}

	// --- Modify GPU Allocations (fine-grained, partial GPU allocation) ---
	if len(sampleServers) > 0 {
		fmt.Printf("Modifying GPU allocations on %s (%s)...\n", tenantName, label)

		// Example: allocate specific GPUs (G0, G1, G2, G3) to a specific server.
		// This is more granular than AllocateGPUs which allocates all GPUs on a server.
		req := ones_gfx.GPUAllocationRequest{
			Operation: ones_gfx.OperationAdd,
			Suid: map[string]map[string]ones_gfx.ServerGPUs{
				"0": {
					"hgx-su00-h00": {GPUs: []string{"G0", "G1", "G2", "G3"}},
				},
			},
		}

		resp, err := client.Fabrics.ModifyGPUAllocations(ctx, fabricName, tenantName, req)
		if err != nil {
			fmt.Printf("  -> modify GPU allocations error: %v\n", err)
		} else {
			fmt.Printf("  -> status: %s\n", resp.Status)
			if resp.OperationID != "" {
				fmt.Printf("  -> operation id: %s (async)\n", resp.OperationID)
				// In real usage, poll the operation to completion if needed
			}
			if resp.Message != "" {
				fmt.Printf("  -> message: %s\n", resp.Message)
			}
		}
	}

	// --- Delete ---
	fmt.Printf("Deleting tenant %q (%s)...\n", tenantName, label)

	if isAsync(mode) {
		var opts []ones_gfx.CallOption
		if isWebhook(mode) {
			opts = append(opts, ones_gfx.WithWebhook(webhookURL, []string{"tenant.delete"}))
		}
		op, err := client.Tenants.DeleteAsync(ctx, fabricName, tenantName, opts...)
		if err != nil {
			fmt.Printf("  -> delete error: %v\n", err)
			return
		}
		fmt.Printf("  -> operation id: %s\n", op.ID)
		pollFn(mode)(client, op.ID, "delete", 360, 5)
	} else {
		err := client.Tenants.Delete(ctx, fabricName, tenantName)
		if err != nil {
			fmt.Printf("  -> delete error: %v\n", err)
			return
		}
		fmt.Println("  -> delete done")
	}
}

// ---------------------------------------------------------------------------
// Scenario 3: individual tenant action.
// ---------------------------------------------------------------------------

func parseServers(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func reportSDKError(err error) {
	var apiErr *ones_gfx.APIError
	if errors.As(err, &apiErr) {
		fmt.Printf("[SDK error] %v (status=%d)\n", err, apiErr.StatusCode)
	} else {
		fmt.Printf("[SDK error] %v\n", err)
	}
}

func scenarioTenantAction(
	client *sdk.Client,
	mode string,
	action string,
	tenantNameOverride string,
	servers []string,
	shared bool,
	peeringNameOverride string,
	vpcNameOverride string,
	peerVPCNameOverride string,
) {
	label := modeLabel(mode)
	ctx := context.Background()

	tName := tenantNameOverride
	if tName == "" {
		tName = tenantNameForMode(mode)
	}
	srvs := servers
	if len(srvs) == 0 {
		srvs = sampleServers
	}

	if action == "read-only" {
		scenarioReadOnly(client)
		return
	}

	fmt.Printf("\n--- Scenario: tenant %s (%s) ---\n", action, label)

	switch action {
	case "create":
		fmt.Printf("Creating tenant %q (%s)...\n", tName, label)
		createReq := resources.CreateTenantRequest{
			Name:           tName,
			Description:    fmt.Sprintf("Created by SDK example (%s mode)", label),
			MaxGPUsAllowed: 8,
		}

		if isAsync(mode) {
			var opts []ones_gfx.CallOption
			if isWebhook(mode) {
				opts = append(opts, ones_gfx.WithWebhook(webhookURL, []string{"tenant.create"}))
			}
			op, err := client.Tenants.CreateAsync(ctx, fabricName, createReq, opts...)
			if err != nil {
				reportSDKError(err)
				return
			}
			fmt.Printf("  -> operation id: %s\n", op.ID)
			if isWebhook(mode) {
				fmt.Printf("  -> webhook registered: %v\n", op.WebhookRegistered)
			}
			pollFn(mode)(client, op.ID, "create", 360, 5)
		} else {
			tenant, err := client.Tenants.Create(ctx, fabricName, createReq)
			if err != nil {
				reportSDKError(err)
				return
			}
			vlanID, vniID := 0, 0
			if tenant.VLANID != nil {
				vlanID = *tenant.VLANID
			}
			if tenant.VNIID != nil {
				vniID = *tenant.VNIID
			}
			fmt.Printf("  -> id=%d, vlan=%d, vni=%d\n", tenant.ID, vlanID, vniID)
		}

	case "allocate":
		if len(srvs) == 0 {
			fmt.Println("No servers provided for allocate. Use --servers or update sampleServers.")
			return
		}
		fmt.Printf("Allocating GPUs %v to %s (%s, shared=%v)...\n", srvs, tName, label, shared)
		serverSpecs := make([]resources.ServerSpec, 0, len(srvs))
		for _, serverName := range srvs {
			serverSpecs = append(serverSpecs, resources.ServerSpec{ServerName: serverName, Shared: shared})
		}
		timeout500 := 500 * time.Second

		if isAsync(mode) {
			var opts []ones_gfx.CallOption
			opts = append(opts, ones_gfx.WithTimeout(timeout500))
			if isWebhook(mode) {
				opts = append(opts, ones_gfx.WithWebhook(webhookURL, []string{"tenant.allocate"}))
			}
			op, err := client.Tenants.AllocateGPUsAsync(ctx, fabricName, tName, serverSpecs, opts...)
			if err != nil {
				reportSDKError(err)
				return
			}
			fmt.Printf("  -> operation id: %s\n", op.ID)
			pollFn(mode)(client, op.ID, "allocate", 360, 5)
		} else {
			err := client.Tenants.AllocateGPUs(ctx, fabricName, tName, serverSpecs, ones_gfx.WithTimeout(timeout500))
			if err != nil {
				reportSDKError(err)
				return
			}
			fmt.Println("  -> allocate done")
		}

	case "deallocate":
		if len(srvs) == 0 {
			fmt.Println("No servers provided for deallocate. Use --servers or update sampleServers.")
			return
		}
		fmt.Printf("Deallocating GPUs %v (%s, shared=%v)...\n", srvs, label, shared)
		serverSpecs := make([]resources.ServerSpec, 0, len(srvs))
		for _, serverName := range srvs {
			serverSpecs = append(serverSpecs, resources.ServerSpec{ServerName: serverName, Shared: shared})
		}
		timeout500 := 500 * time.Second

		if isAsync(mode) {
			var opts []ones_gfx.CallOption
			opts = append(opts, ones_gfx.WithTimeout(timeout500))
			if isWebhook(mode) {
				opts = append(opts, ones_gfx.WithWebhook(webhookURL, []string{"tenant.deallocate"}))
			}
			op, err := client.Tenants.DeallocateGPUsAsync(ctx, fabricName, tName, serverSpecs, opts...)
			if err != nil {
				reportSDKError(err)
				return
			}
			fmt.Printf("  -> operation id: %s\n", op.ID)
			pollFn(mode)(client, op.ID, "deallocate", 360, 5)
		} else {
			err := client.Tenants.DeallocateGPUs(ctx, fabricName, tName, serverSpecs, ones_gfx.WithTimeout(timeout500))
			if err != nil {
				reportSDKError(err)
				return
			}
			fmt.Println("  -> deallocate done")
		}

	case "delete":
		fmt.Printf("Deleting tenant %q (%s)...\n", tName, label)

		if isAsync(mode) {
			var opts []ones_gfx.CallOption
			if isWebhook(mode) {
				opts = append(opts, ones_gfx.WithWebhook(webhookURL, []string{"tenant.delete"}))
			}
			op, err := client.Tenants.DeleteAsync(ctx, fabricName, tName, opts...)
			if err != nil {
				reportSDKError(err)
				return
			}
			fmt.Printf("  -> operation id: %s\n", op.ID)
			pollFn(mode)(client, op.ID, "delete", 360, 5)
		} else {
			err := client.Tenants.Delete(ctx, fabricName, tName)
			if err != nil {
				reportSDKError(err)
				return
			}
			fmt.Println("  -> delete done")
		}

	case "vpcpeering":
		if mode != "sync" {
			fmt.Println("VPC peering is supported only in sync mode.")
			return
		}
		vName := vpcNameOverride
		if vName == "" {
			vName = defaultVPCName(tName)
		}
		pvName := peerVPCNameOverride
		if pvName == "" {
			pvName = defaultPeerVPCName()
		}
		pName := peeringNameOverride
		if pName == "" {
			pName = fmt.Sprintf("%s-storage-route-leak", tName)
		}
		fmt.Printf("Creating VPC peering %q between %q and %q...\n", pName, vName, pvName)
		result, err := client.Peering.Create(ctx, fabricName, pName, vName, pvName)
		if err != nil {
			reportSDKError(err)
			return
		}
		fmt.Printf("  -> response: %v\n", result)

	default:
		fmt.Printf("Unknown action: %s\n", action)
	}
}

// ---------------------------------------------------------------------------
// Scenario 5: error handling.
// ---------------------------------------------------------------------------

func scenarioErrorHandling(client *sdk.Client) {
	fmt.Println("\n--- Scenario: error handling ---")
	ctx := context.Background()

	// Invalid input — caught client-side before any HTTP call.
	_, err := client.Tenants.Create(ctx, fabricName, resources.CreateTenantRequest{
		Name:           "bad_quota_demo",
		Description:    "will not be sent",
		MaxGPUsAllowed: 0, // disallowed: must be -1 or >= 1
	})
	if err != nil {
		fmt.Printf("[client-side validation] %v\n", err)
	}

	// Server-side 404 — looking up a tenant that doesn't exist.
	_, err = client.Tenants.Get(ctx, fabricName, "this_tenant_does_not_exist_xyz")
	if err != nil {
		var notFoundErr *ones_gfx.NotFoundError
		if errors.As(err, &notFoundErr) {
			fmt.Printf("[NotFoundError] %v (status=%d)\n", notFoundErr, notFoundErr.StatusCode)
		} else {
			var onesErr ones_gfx.ONESError
			if errors.As(err, &onesErr) {
				fmt.Printf("[ONESError] %v\n", onesErr)
			} else {
				fmt.Printf("[error] %v\n", err)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Entry point.
// ---------------------------------------------------------------------------

func main() {
	// Turn on debug logging if you want to see the SDK's internal HTTP
	// activity (token refreshes, retry-on-401, etc.).
	// log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Load configuration from .env file and environment variables
	if err := loadEnv(); err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	mode := flag.String("mode", "sync", "Tenant lifecycle mode: sync, async-poll, async-webhook")
	action := flag.String("action", "lifecycle", "Action: lifecycle, read-only, login, create, allocate, deallocate, delete, vpcpeering, gpu-allocations")
	tenantNameFlag := flag.String("tenant-name", "", "Override tenant name for create/delete/allocate/deallocate")
	username := flag.String("username", "", "Username for login action (default: loginUsername)")
	password := flag.String("password", "", "Password for login action (default: loginPassword)")
	serversFlag := flag.String("servers", "", "Comma-separated server list for allocate/deallocate")
	sharedFlag := flag.Bool("shared", false, "Set shared on allocate/deallocate server specs (e.g. --shared=true)")
	peeringNameFlag := flag.String("peering-name", "", "Peering name for vpcpeering (default: <tenant>-storage-route-leak)")
	vpcNameFlag := flag.String("vpc-name", "", "Tenant VPC name for vpcpeering (default: <tenant>-<fabric>-north-south)")
	peerVPCNameFlag := flag.String("peer-vpc-name", "", "Peer VPC name for vpcpeering (default: <fabric>-Storage-VPC)")
	gpuOperation := flag.String("gpu-operation", "ADD", "GPU allocation operation: ADD or DELETE (used with --action gpu-allocations)")
	gpuHostname := flag.String("gpu-hostname", "", "Compute node hostname for gpu-allocations (e.g. hgx-su00-h00)")
	gpuIDs := flag.String("gpu-ids", "G0,G1,G2,G3", "Comma-separated GPU IDs for gpu-allocations (e.g. G0,G1,G2,G3)")
	gpuServerIndex := flag.String("gpu-server-index", "0", "Server index key in suid map (default: 0)")
	flag.Parse()

	// Validate mode
	validModes := map[string]bool{"sync": true, "async-poll": true, "async-webhook": true}
	if !validModes[*mode] {
		log.Fatalf("invalid mode %q; must be sync, async-poll, or async-webhook", *mode)
	}

	// Validate action
	validActions := map[string]bool{
		"lifecycle": true, "read-only": true, "login": true,
		"create": true, "allocate": true, "deallocate": true,
		"delete": true, "vpcpeering": true, "gpu-allocations": true,
	}
	if !validActions[*action] {
		log.Fatalf("invalid action %q", *action)
	}

	if *action == "login" {
		u := *username
		if u == "" {
			u = loginUsername
		}
		p := *password
		if p == "" {
			p = loginPassword
		}
		scenarioLogin(u, p)
		return
	}

	client := buildClient()
	defer client.Close()

	if *action == "lifecycle" {
		scenarioReadOnly(client)
		scenarioTenantLifecycle(client, *mode)
		scenarioErrorHandling(client)
	} else if *action == "gpu-allocations" {
		tName := *tenantNameFlag
		if tName == "" {
			log.Fatal("--tenant-name is required for gpu-allocations")
		}
		hostname := *gpuHostname
		if hostname == "" {
			if len(sampleServers) > 0 {
				hostname = sampleServers[0]
			} else {
				log.Fatal("--gpu-hostname is required (or set sampleServers in config)")
			}
		}
		scenarioGPUAllocations(client, tName, *gpuOperation, *gpuServerIndex, hostname, parseServers(*gpuIDs))
	} else {
		scenarioTenantAction(
			client,
			*mode,
			*action,
			*tenantNameFlag,
			parseServers(*serversFlag),
			*sharedFlag,
			*peeringNameFlag,
			*vpcNameFlag,
			*peerVPCNameFlag,
		)
	}
}

// scenarioGPUAllocations calls POST /fabrics/{fabric}/tenants/{tenant}/gpuAllocations
// with the given operation, server index, hostname, and GPU IDs.
func scenarioGPUAllocations(client *sdk.Client, tenantName, operation, serverIndex, hostname string, gpuIDs []string) {
	fmt.Printf("\n--- Scenario: gpu-allocations ---\n")
	fmt.Printf("  fabric:   %s\n", fabricName)
	fmt.Printf("  tenant:   %s\n", tenantName)
	fmt.Printf("  op:       %s\n", operation)
	fmt.Printf("  suid[%s][%s].gpus: %v\n", serverIndex, hostname, gpuIDs)

	ctx := context.Background()
	req := ones_gfx.GPUAllocationRequest{
		Operation: ones_gfx.GPUOperation(operation),
		Suid: map[string]map[string]ones_gfx.ServerGPUs{
			serverIndex: {
				hostname: {GPUs: gpuIDs},
			},
		},
	}

	resp, err := client.Fabrics.ModifyGPUAllocations(ctx, fabricName, tenantName, req)
	if err != nil {
		reportSDKError(err)
		return
	}
	fmt.Printf("  -> status: %s\n", resp.Status)
	if resp.OperationID != "" {
		fmt.Printf("  -> operation id: %s\n", resp.OperationID)
	}
	if resp.Message != "" {
		fmt.Printf("  -> message: %s\n", resp.Message)
	}
}
