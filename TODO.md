# Spec: Auto-Publish Per-Test Grafana Dashboards from Orderly Ape

## 1. Overview  
When an Orderly Ape benchmark run starts and completes, automatically:  
1. **Real-time Dashboard**: Create a live-updating dashboard that stakeholders can watch during test execution
2. **Results Dashboard**: Clone the existing "test-results" Grafana dashboard template with final results
3. Inject this run's `testid` as a hidden constant variable  
4. Place both dashboards in publicly-viewable folders  
5. Return shareable URLs for both live monitoring and final results

Anonymous users can open either link, interact with panels and time filters, and see **only** that test's data - either live during execution or final results after completion.

---

## 2. Prerequisites  
- **Grafana 8.6.4+** with HTTP API enabled (already deployed via Helm)
- **Admin password authentication** (already configured)
- **Grafana API token** with `Editor` scope (to be created)
- The existing "test-results" dashboard template in `grafana/test-results.json` (already contains proper `testid` variable)
- Two Grafana folders:
  - **Live Tests** (UID=`live-tests`) - for real-time monitoring
  - **Public Tests** (UID=`public-tests`) - for final results
- Grafana's folder permissions configured so Anonymous (`userId=0`) has Viewer role **only** on these folders

---

## 3. Configuration  
Add to your Orderly Ape config (e.g. `config.yaml`):

```yaml
grafana:
  base_url:      "https://grafana.example.com"
  # Admin authentication (existing)
  admin_username: "admin"
  admin_password: "<YOUR_GRAFANA_ADMIN_PASSWORD>"
  # API token for dashboard operations (new)
  api_token:     "<YOUR_GRAFANA_API_TOKEN>"
  org_id:        1
  folders:
    live_tests:    "live-tests"      # Real-time monitoring
    public_tests:  "public-tests"    # Final results
  template_path: "./grafana/test-results.json"
  # Optional: retention and validation
  retention_days: 30
  validate_template: true
  # Real-time dashboard settings
  live_dashboard:
    enabled: true
    refresh_interval: "5s"           # How often to refresh live data
    auto_cleanup: true               # Remove live dashboard after test completion
```

# Grafana Dashboard Implementation Guide

## Implementation Steps

### 4.1 API Token Setup (One-Time)

#### 1. Create API Token via Grafana UI or API

**Option A: Via Grafana UI**
1. Log into Grafana as admin
2. Go to Configuration → API Keys
3. Create new API key with "Editor" role
4. Copy the token

**Option B: Via API (automated)**
```bash
curl -X POST \
     -H "Content-Type: application/json" \
     -u admin:<ADMIN_PASSWORD> \
     "$GRAFANA_BASE_URL/api/auth/keys" \
     -d '{
       "name": "orderly-ape-dashboard-creator",
       "role": "Editor"
     }'
```

#### 2. Store Token Securely
Add the token to your Kubernetes secrets or environment variables:
```bash
# Add to helmfile.yaml values
grafana:
  api_token: "<GENERATED_TOKEN>"
```

### 4.2 Folder Setup (One-Time)

#### 1. Create "Live Tests" and "Public Tests" folders

```bash
# Create Live Tests folder
curl -X POST \
     -H "Authorization: Bearer $GRAFANA_API_TOKEN" \
     -H "Content-Type: application/json" \
     "$GRAFANA_BASE_URL/api/folders" \
     -d '{ "title": "Live Tests", "uid": "live-tests" }'

# Create Public Tests folder
curl -X POST \
     -H "Authorization: Bearer $GRAFANA_API_TOKEN" \
     -H "Content-Type: application/json" \
     "$GRAFANA_BASE_URL/api/folders" \
     -d '{ "title": "Public Tests", "uid": "public-tests" }'
```

#### 2. Grant Viewer access to Anonymous for both folders

Replace `<liveFolderId>` and `<publicFolderId>` with the returned IDs:

```bash
# Grant access to Live Tests folder
curl -X POST \
     -H "Authorization: Bearer $GRAFANA_API_TOKEN" \
     -H "Content-Type: application/json" \
     "$GRAFANA_BASE_URL/api/folders/<liveFolderId>/permissions" \
     -d '[
         {
           "userId": 0,
           "permission": 1
         }
     ]'

# Grant access to Public Tests folder
curl -X POST \
     -H "Authorization: Bearer $GRAFANA_API_TOKEN" \
     -H "Content-Type: application/json" \
     "$GRAFANA_BASE_URL/api/folders/<publicFolderId>/permissions" \
     -d '[
         {
           "userId": 0,
           "permission": 1
         }
     ]'
```

### 4.3 Real-Time Dashboard Creation (Test Start)

Create a live dashboard when the test starts:

```go
// CreateLiveDashboard creates a real-time dashboard for monitoring test progress
func CreateLiveDashboard(testID string) (string, error) {
  // 1. Validate template exists and is valid
  if err := validateTemplate(config.Grafana.TemplatePath); err != nil {
    return "", fmt.Errorf("template validation failed: %w", err)
  }

  // 2. Load master template JSON
  tpl, err := os.ReadFile(config.Grafana.TemplatePath)
  if err != nil {
    return "", fmt.Errorf("failed to read template: %w", err)
  }

  // 3. Unmarshal into map
  var dash map[string]interface{}
  if err := json.Unmarshal(tpl, &dash); err != nil {
    return "", fmt.Errorf("failed to parse template JSON: %w", err)
  }

  // 4. Modify for live monitoring
  dash["title"] = fmt.Sprintf("Live Test #%s", testID)
  dash["uid"]   = fmt.Sprintf("live-test-%s", testID)
  
  // 5. Set live refresh interval
  dash["refresh"] = config.Grafana.LiveDashboard.RefreshInterval
  
  // 6. Override templating to one hidden constant variable
  dash["templating"] = map[string]interface{}{
    "list": []interface{}{
      map[string]interface{}{
        "name":  "testid",        // ✅ Correct variable name
        "type":  "constant",
        "hide":  2,               // 2 = hidden
        "query": testID,
      },
    },
  }

  // 7. Ensure Live Tests folder exists and get its ID
  folderID, err := getOrCreateFolder(config.Grafana.Folders.LiveTests, "Live Tests")
  if err != nil {
    return "", fmt.Errorf("folder setup failed: %w", err)
  }

  // 8. Build payload
  payload := map[string]interface{}{
    "dashboard": dash,
    "folderId":  folderID,
    "overwrite": true,
  }

  // 9. POST to Grafana using API token
  resp, err := grafanaAPIRequest("POST", "/api/dashboards/db", payload)
  if err != nil {
    return "", fmt.Errorf("live dashboard creation failed: %w", err)
  }

  // 10. Extract UID and build URL
  body := resp.(map[string]interface{})
  uid  := body["uid"].(string)
  url  := fmt.Sprintf("%s/d/%s?orgId=%d",
          config.Grafana.BaseURL, uid, config.Grafana.OrgID)

  return url, nil
}
```

### 4.4 Final Results Dashboard Creation (Test Completion)

Create the final results dashboard when the test completes:

```go
// PublishGrafanaDashboard creates a final results dashboard
func PublishGrafanaDashboard(testID string) (string, error) {
  // 1. Validate template exists and is valid
  if err := validateTemplate(config.Grafana.TemplatePath); err != nil {
    return "", fmt.Errorf("template validation failed: %w", err)
  }

  // 2. Load master template JSON
  tpl, err := os.ReadFile(config.Grafana.TemplatePath)
  if err != nil {
    return "", fmt.Errorf("failed to read template: %w", err)
  }

  // 3. Unmarshal into map
  var dash map[string]interface{}
  if err := json.Unmarshal(tpl, &dash); err != nil {
    return "", fmt.Errorf("failed to parse template JSON: %w", err)
  }

  // 4. Inject test-specific metadata
  dash["title"] = fmt.Sprintf("Test Results #%s", testID)
  dash["uid"]   = fmt.Sprintf("test-%s", testID)
  
  // 5. Set appropriate refresh for final results
  dash["refresh"] = "1m"  // Refresh every minute for final results

  // 6. Override templating to one hidden constant variable
  dash["templating"] = map[string]interface{}{
    "list": []interface{}{
      map[string]interface{}{
        "name":  "testid",        // ✅ Correct variable name
        "type":  "constant",
        "hide":  2,               // 2 = hidden
        "query": testID,
      },
    },
  }

  // 7. Ensure Public Tests folder exists and get its ID
  folderID, err := getOrCreateFolder(config.Grafana.Folders.PublicTests, "Public Tests")
  if err != nil {
    return "", fmt.Errorf("folder setup failed: %w", err)
  }

  // 8. Build payload
  payload := map[string]interface{}{
    "dashboard": dash,
    "folderId":  folderID,
    "overwrite": true,
  }

  // 9. POST to Grafana using API token
  resp, err := grafanaAPIRequest("POST", "/api/dashboards/db", payload)
  if err != nil {
    return "", fmt.Errorf("dashboard creation failed: %w", err)
  }

  // 10. Extract UID and build URL
  body := resp.(map[string]interface{})
  uid  := body["uid"].(string)
  url  := fmt.Sprintf("%s/d/%s?orgId=%d",
          config.Grafana.BaseURL, uid, config.Grafana.OrgID)

  return url, nil
}
```

### 4.5 Dashboard Cleanup (Test Completion)

Clean up the live dashboard after test completion:

```go
// CleanupLiveDashboard removes the live dashboard after test completion
func CleanupLiveDashboard(testID string) error {
  if !config.Grafana.LiveDashboard.AutoCleanup {
    return nil // Skip cleanup if disabled
  }

  dashboardUID := fmt.Sprintf("live-test-%s", testID)
  
  // Delete the live dashboard
  _, err := grafanaAPIRequest("DELETE", fmt.Sprintf("/api/dashboards/uid/%s", dashboardUID), nil)
  if err != nil {
    return fmt.Errorf("failed to cleanup live dashboard: %w", err)
  }

  log.Printf("Cleaned up live dashboard for test %s", testID)
  return nil
}
```

### 4.6 Integrate into Test Runner

Integrate both dashboards into the test workflow:

```go
// Test start - create live dashboard
func onTestStart(testID string) {
  if config.Grafana.LiveDashboard.Enabled {
    liveURL, err := CreateLiveDashboard(testID)
    if err != nil {
      log.Printf("Failed to create live dashboard: %v", err)
    } else {
      fmt.Printf("Watch test live: %s\n", liveURL)
      // Store URL for later cleanup
      setLiveDashboardURL(testID, liveURL)
    }
  }
}

// Test completion - create final dashboard and cleanup
func onTestComplete(testID string) {
  // Create final results dashboard
  resultsURL, err := PublishGrafanaDashboard(testID)
  if err != nil {
    log.Printf("Failed to publish results dashboard: %v", err)
  } else {
    fmt.Printf("View final results: %s\n", resultsURL)
  }

  // Cleanup live dashboard
  if config.Grafana.LiveDashboard.Enabled {
    if err := CleanupLiveDashboard(testID); err != nil {
      log.Printf("Failed to cleanup live dashboard: %v", err)
    }
  }
}
```

#### Helper Functions Required

* `validateTemplate(templatePath string) error`
  * Check that template exists and is valid JSON
  * Verify it contains a `testid` variable
  * Ensure all panels reference `testid` in queries
  * Validate required fields (title, uid, etc.)

* `getOrCreateFolder(folderUID, title string) (int, error)`
  * GET `/api/folders/:uid` → extract `id`
  * If not found, POST `/api/folders` to create
  * Set up anonymous permissions if needed

* `grafanaAPIRequest(method, path string, body interface{}) (interface{}, error)`
  * Wraps HTTP client, sets headers:
    * `Authorization: Bearer <api_token>`
    * `Content-Type: application/json`
  * Encodes `body` to JSON, sends, decodes JSON response

* `setLiveDashboardURL(testID, url string)` and `getLiveDashboardURL(testID string)`
  * Store/retrieve live dashboard URLs for cleanup

### 4.7 Dashboard Cleanup (Optional)

Implement cleanup for old dashboards:

```go
func CleanupOldDashboards(retentionDays int) error {
  // Find dashboards older than retention_days
  // Delete them via API
  // Log cleanup actions
  return nil
}
```

## 5. Security Considerations

### 5.1 Authentication Strategy
- **Admin Password**: Used for initial setup and token creation
- **API Token**: Used for dashboard operations (more secure for automation)
- **Anonymous Access**: Limited to "Live Tests" and "Public Tests" folders only

### 5.2 Data Isolation
- All dashboard queries filter by `testid` tag
- Anonymous users cannot access other dashboards or data
- Template variables are hidden to prevent manipulation
- Live dashboards are automatically cleaned up after test completion

### 5.3 Token Management
- Store API tokens securely (Kubernetes secrets)
- Rotate tokens periodically
- Use minimal required permissions (Editor role)

### 5.4 Real-Time Security
- Live dashboards only show data for the specific test
- Automatic cleanup prevents data leakage
- Configurable refresh intervals to control data exposure

## 6. Deployment Integration

### 6.1 Update Helmfile Configuration

Add to `deploy/all-in-one/helmfile.yaml`:

```yaml
grafana:
  # Existing configuration...
  adminPassword: {{ .Values.grafana.adminPassword | quote }}
  
  # New API token configuration
  apiToken: {{ .Values.grafana.apiToken | quote }}
  
  # Folder and permission setup
  folders:
    live-tests:
      title: "Live Tests"
      uid: "live-tests"
      permissions:
        anonymous:
          role: "Viewer"
    public-tests:
      title: "Public Tests"
      uid: "public-tests"
      permissions:
        anonymous:
          role: "Viewer"
```

### 6.2 Environment Variables

Add to orderly-ape deployment:

```yaml
env:
  - name: GRAFANA_API_TOKEN
    valueFrom:
      secretKeyRef:
        name: grafana-auth
        key: api-token
  - name: GRAFANA_LIVE_DASHBOARD_ENABLED
    value: "true"
  - name: GRAFANA_LIVE_DASHBOARD_REFRESH
    value: "5s"
```

## Acceptance Criteria

* **Live Dashboard**: A dashboard titled **Live Test #\<id\>** (UID `live-test-<id>`) appears under **Live Tests** when test starts.
* **Results Dashboard**: A dashboard titled **Test Results #\<id\>** (UID `test-<id>`) appears under **Public Tests** when test completes.
* **Real-Time Updates**: Live dashboard refreshes every 5 seconds during test execution.
* **Data Isolation**: Panels only display data where `testid == <id>` (verified by existing template).
* **Anonymous Access**: Users can view both dashboards without credentials.
* **Automatic Cleanup**: Live dashboard is automatically removed after test completion.
* **URL Generation**: Orderly Ape outputs valid links for both dashboards:

```
Watch test live: https://grafana.example.com/d/live-test-12345?orgId=1
View final results: https://grafana.example.com/d/test-12345?orgId=1
```

* **Non-blocking**: Failed dashboard creation doesn't break the test run.

## Testing Strategy

### 7.1 Template Validation
- Verify all queries in template use `testid` filtering
- Test with sample data to ensure isolation works
- Validate template JSON structure

### 7.2 Authentication Testing
- Test API token creation and usage
- Verify admin password still works for UI access
- Test anonymous access to both folders

### 7.3 Real-Time Testing
- Start a test and verify live dashboard creation
- Monitor real-time data updates during test execution
- Verify automatic cleanup after test completion

### 7.4 Integration Testing
- Run complete test workflow with both dashboards
- Verify URL generation and accessibility
- Test data isolation with multiple concurrent tests

## Use Cases

### 7.1 Stakeholder Monitoring
- **Product Managers**: Watch test progress in real-time during load testing
- **Developers**: Monitor system performance as tests run
- **QA Teams**: Observe test execution without needing credentials

### 7.2 Post-Test Analysis
- **Architects**: Review final results and performance metrics
- **Operations**: Analyze system behavior under load
- **Business Teams**: Share results with stakeholders

## References

* [Grafana HTTP API (Dashboards)](https://grafana.com/docs/grafana/latest/http_api/dashboard/#create-or-update-dashboard)
* [Grafana HTTP API (Folders & Permissions)](https://grafana.com/docs/grafana/latest/http_api/folder/)
* [Grafana HTTP API (API Keys)](https://grafana.com/docs/grafana/latest/http_api/auth/)
* [Hide template variables in JSON: Set `"hide": 2`](https://grafana.com/docs/grafana/latest/dashboards/variables/add-template-variables/#variable-options)
* [Grafana Dashboard Refresh Settings](https://grafana.com/docs/grafana/latest/dashboards/manage-dashboards/#dashboard-settings)
* Existing template: `grafana/test-results.json` (already implements proper `testid` filtering)