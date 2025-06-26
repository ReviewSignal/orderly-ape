# Spec: Auto-Publish Per-Test Grafana Dashboards from Orderly Ape

## 1. Overview  
When an Orderly Ape benchmark run starts and completes, users can optionally:  
1. **Live Monitoring**: Create a real-time dashboard that stakeholders can watch during test execution (user choice)
2. **Results Dashboard**: Create a final results dashboard for post-test analysis (user choice)
3. **Manual Creation**: Create dashboards for existing completed tests (user choice)
4. Inject the test's `testid` as a hidden constant variable  
5. Place dashboards in a publicly-viewable folder  
6. Return shareable URLs for monitoring and results

Anonymous users can open the links, interact with panels and time filters, and see **only** that test's data. Dashboards persist indefinitely unless the user specifies an expiry time.

---

## 2. Prerequisites  
- **Grafana 8.6.4+** with HTTP API enabled (already deployed via Helm)
- **Admin password authentication** (already configured)
- **Grafana API token** with `Editor` scope (to be created)
- The existing "test-results" dashboard template in `grafana/test-results.json` (already contains proper `testid` variable)
- A Grafana folder named **Public Tests** (UID=`public-tests`) to be created
- Grafana's folder permissions configured so Anonymous (`userId=0`) has Viewer role **only** on this folder

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
  folder_uid:    "public-tests"
  template_path: "./grafana/test-results.json"
  # Optional: validation
  validate_template: true
  # Dashboard settings
  dashboard:
    # Default refresh intervals
    live_refresh: "5s"              # During test execution
    results_refresh: "1m"           # For completed tests
    # Default retention (null = no expiry)
    default_retention_days: null    # Dashboards live forever by default
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

#### 1. Create "Public Tests" folder

```bash
curl -X POST \
     -H "Authorization: Bearer $GRAFANA_API_TOKEN" \
     -H "Content-Type: application/json" \
     "$GRAFANA_BASE_URL/api/folders" \
     -d '{ "title": "Public Tests", "uid": "public-tests" }'
```

#### 2. Grant Viewer access to Anonymous

Replace `<folderId>` with the returned ID:

```bash
curl -X POST \
     -H "Authorization: Bearer $GRAFANA_API_TOKEN" \
     -H "Content-Type: application/json" \
     "$GRAFANA_BASE_URL/api/folders/<folderId>/permissions" \
     -d '[
         {
           "userId": 0,
           "permission": 1
         }
     ]'
```

### 4.3 Dashboard Creation Functions

#### Create Dashboard (Generic Function)

```go
// CreateDashboard creates a dashboard with specified settings
func CreateDashboard(testID string, options DashboardOptions) (string, error) {
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

  // 4. Set dashboard metadata
  dash["title"] = options.Title
  dash["uid"]   = options.UID
  dash["refresh"] = options.RefreshInterval

  // 5. Override templating to one hidden constant variable
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

  // 6. Ensure folder exists and get its ID
  folderID, err := getOrCreateFolder(config.Grafana.FolderUID, "Public Tests")
  if err != nil {
    return "", fmt.Errorf("folder setup failed: %w", err)
  }

  // 7. Build payload
  payload := map[string]interface{}{
    "dashboard": dash,
    "folderId":  folderID,
    "overwrite": true,
  }

  // 8. POST to Grafana using API token
  resp, err := grafanaAPIRequest("POST", "/api/dashboards/db", payload)
  if err != nil {
    return "", fmt.Errorf("dashboard creation failed: %w", err)
  }

  // 9. Extract UID and build URL
  body := resp.(map[string]interface{})
  uid  := body["uid"].(string)
  url  := fmt.Sprintf("%s/d/%s?orgId=%d",
          config.Grafana.BaseURL, uid, config.Grafana.OrgID)

  // 10. Schedule cleanup if retention is specified
  if options.RetentionDays > 0 {
    scheduleDashboardCleanup(uid, options.RetentionDays)
  }

  return url, nil
}

// DashboardOptions defines dashboard creation parameters
type DashboardOptions struct {
  Title           string
  UID             string
  RefreshInterval string
  RetentionDays   int // 0 = no expiry
}
```

#### Create Live Dashboard (During Test Execution)

```go
// CreateLiveDashboard creates a real-time dashboard for monitoring test progress
func CreateLiveDashboard(testID string, retentionDays int) (string, error) {
  options := DashboardOptions{
    Title:           fmt.Sprintf("Live Test #%s", testID),
    UID:             fmt.Sprintf("live-test-%s", testID),
    RefreshInterval: config.Grafana.Dashboard.LiveRefresh,
    RetentionDays:   retentionDays,
  }
  
  return CreateDashboard(testID, options)
}
```

#### Create Results Dashboard (Post-Test)

```go
// CreateResultsDashboard creates a final results dashboard
func CreateResultsDashboard(testID string, retentionDays int) (string, error) {
  options := DashboardOptions{
    Title:           fmt.Sprintf("Test Results #%s", testID),
    UID:             fmt.Sprintf("test-%s", testID),
    RefreshInterval: config.Grafana.Dashboard.ResultsRefresh,
    RetentionDays:   retentionDays,
  }
  
  return CreateDashboard(testID, options)
}
```

### 4.4 Dashboard Management Functions

#### List Dashboards for a Test

```go
// ListTestDashboards returns all dashboards for a specific test
func ListTestDashboards(testID string) ([]DashboardInfo, error) {
  // Search for dashboards with testID in title or UID
  resp, err := grafanaAPIRequest("GET", "/api/search?query="+testID, nil)
  if err != nil {
    return nil, fmt.Errorf("failed to search dashboards: %w", err)
  }

  var dashboards []DashboardInfo
  // Parse response and filter for test-specific dashboards
  // Return dashboard info including UID, title, URL, creation time
  
  return dashboards, nil
}

type DashboardInfo struct {
  UID         string
  Title       string
  URL         string
  Created     time.Time
  ExpiresAt   *time.Time // nil if no expiry
}
```

#### Delete Dashboard

```go
// DeleteDashboard removes a specific dashboard
func DeleteDashboard(dashboardUID string) error {
  _, err := grafanaAPIRequest("DELETE", fmt.Sprintf("/api/dashboards/uid/%s", dashboardUID), nil)
  if err != nil {
    return fmt.Errorf("failed to delete dashboard: %w", err)
  }
  
  log.Printf("Deleted dashboard %s", dashboardUID)
  return nil
}
```

#### Update Dashboard Retention

```go
// UpdateDashboardRetention changes the expiry time for a dashboard
func UpdateDashboardRetention(dashboardUID string, retentionDays int) error {
  if retentionDays <= 0 {
    // Remove expiry - dashboard lives forever
    cancelDashboardCleanup(dashboardUID)
  } else {
    // Set new expiry
    scheduleDashboardCleanup(dashboardUID, retentionDays)
  }
  
  return nil
}
```

### 4.5 User Interface Integration

#### Test Launch Options (Webapp)

```go
// TestLaunchOptions defines user choices when starting a test via webapp
type TestLaunchOptions struct {
  CreateLiveDashboard bool
  LiveRetentionDays   int // 0 = no expiry
  
  CreateResultsDashboard bool
  ResultsRetentionDays   int // 0 = no expiry
}

// onTestStart handles dashboard creation when test starts
func onTestStart(testID string, options TestLaunchOptions) {
  if options.CreateLiveDashboard {
    liveURL, err := CreateLiveDashboard(testID, options.LiveRetentionDays)
    if err != nil {
      log.Printf("Failed to create live dashboard: %v", err)
    } else {
      fmt.Printf("Watch test live: %s\n", liveURL)
    }
  }
}
```

#### Test Completion

```go
// onTestComplete handles dashboard creation when test completes
func onTestComplete(testID string, options TestLaunchOptions) {
  if options.CreateResultsDashboard {
    resultsURL, err := CreateResultsDashboard(testID, options.ResultsRetentionDays)
    if err != nil {
      log.Printf("Failed to create results dashboard: %v", err)
    } else {
      fmt.Printf("View final results: %s\n", resultsURL)
    }
  }
}
```

#### Webapp Dashboard Management

```go
// CreateDashboardForCompletedTest allows creating dashboards for existing tests via webapp
func CreateDashboardForCompletedTest(testID string, dashboardType string, retentionDays int) (string, error) {
  switch dashboardType {
  case "live":
    return CreateLiveDashboard(testID, retentionDays)
  case "results":
    return CreateResultsDashboard(testID, retentionDays)
  default:
    return "", fmt.Errorf("invalid dashboard type: %s", dashboardType)
  }
}

// Webapp API endpoints for dashboard management
// POST /api/tests/{test_id}/dashboards - Create dashboard for test
// GET /api/tests/{test_id}/dashboards - List dashboards for test
// DELETE /api/dashboards/{uid} - Delete specific dashboard
// PUT /api/dashboards/{uid}/retention - Update dashboard retention
```

### 4.6 Webapp Integration

#### Dashboard Management UI

The orderly-ape webapp should include dashboard management features:

**Test Launch Page:**
- Checkbox: "Create live dashboard during test"
- Input: "Live dashboard retention (days, 0 = no expiry)"
- Checkbox: "Create results dashboard after test"
- Input: "Results dashboard retention (days, 0 = no expiry)"

**Test Details Page:**
- Section: "Dashboards"
- List existing dashboards for the test
- Button: "Create Live Dashboard"
- Button: "Create Results Dashboard"
- For each dashboard: URL, creation time, expiry time, delete button

**Dashboard Management Page:**
- List all dashboards across all tests
- Filter by test ID, dashboard type, creation date
- Bulk operations: delete multiple, update retention
- Search and pagination

#### Webapp API Endpoints

```python
# Django views for dashboard management

@api_view(['POST'])
def create_test_dashboard(request, test_id):
    """Create a dashboard for a specific test"""
    dashboard_type = request.data.get('type')  # 'live' or 'results'
    retention_days = request.data.get('retention_days', 0)
    
    url, error = CreateDashboardForCompletedTest(test_id, dashboard_type, retention_days)
    if error:
        return Response({'error': str(error)}, status=400)
    
    return Response({'url': url})

@api_view(['GET'])
def list_test_dashboards(request, test_id):
    """List all dashboards for a specific test"""
    dashboards, error = ListTestDashboards(test_id)
    if error:
        return Response({'error': str(error)}, status=400)
    
    return Response({'dashboards': dashboards})

@api_view(['DELETE'])
def delete_dashboard(request, dashboard_uid):
    """Delete a specific dashboard"""
    error = DeleteDashboard(dashboard_uid)
    if error:
        return Response({'error': str(error)}, status=400)
    
    return Response({'success': True})

@api_view(['PUT'])
def update_dashboard_retention(request, dashboard_uid):
    """Update retention period for a dashboard"""
    retention_days = request.data.get('retention_days', 0)
    
    error = UpdateDashboardRetention(dashboard_uid, retention_days)
    if error:
        return Response({'error': str(error)}, status=400)
    
    return Response({'success': True})
```

#### Webapp Templates

**Test Launch Template:**
```html
<!-- Dashboard options section -->
<div class="dashboard-options">
  <h3>Dashboard Options</h3>
  
  <div class="form-group">
    <label>
      <input type="checkbox" name="create_live_dashboard">
      Create live dashboard during test execution
    </label>
    <input type="number" name="live_retention_days" placeholder="Retention days (0 = no expiry)" min="0">
  </div>
  
  <div class="form-group">
    <label>
      <input type="checkbox" name="create_results_dashboard">
      Create results dashboard after test completion
    </label>
    <input type="number" name="results_retention_days" placeholder="Retention days (0 = no expiry)" min="0">
  </div>
</div>
```

**Test Details Template:**
```html
<!-- Dashboard management section -->
<div class="dashboards-section">
  <h3>Dashboards</h3>
  
  <div class="dashboard-actions">
    <button class="btn btn-primary" onclick="createDashboard('live')">
      Create Live Dashboard
    </button>
    <button class="btn btn-primary" onclick="createDashboard('results')">
      Create Results Dashboard
    </button>
  </div>
  
  <div class="dashboard-list">
    {% for dashboard in test.dashboards %}
    <div class="dashboard-item">
      <div class="dashboard-info">
        <strong>{{ dashboard.title }}</strong>
        <a href="{{ dashboard.url }}" target="_blank">View Dashboard</a>
        <span>Created: {{ dashboard.created }}</span>
        {% if dashboard.expires_at %}
        <span>Expires: {{ dashboard.expires_at }}</span>
        {% else %}
        <span>No expiry</span>
        {% endif %}
      </div>
      <div class="dashboard-actions">
        <button class="btn btn-sm btn-warning" onclick="updateRetention('{{ dashboard.uid }}')">
          Update Retention
        </button>
        <button class="btn btn-sm btn-danger" onclick="deleteDashboard('{{ dashboard.uid }}')">
          Delete
        </button>
      </div>
    </div>
    {% endfor %}
  </div>
</div>
```

#### JavaScript for Dashboard Management

```javascript
// Dashboard management functions for webapp

function createDashboard(type) {
  const testId = getCurrentTestId();
  const retentionDays = prompt(`Enter retention days for ${type} dashboard (0 = no expiry):`, "0");
  
  fetch(`/api/tests/${testId}/dashboards`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-CSRFToken': getCSRFToken()
    },
    body: JSON.stringify({
      type: type,
      retention_days: parseInt(retentionDays)
    })
  })
  .then(response => response.json())
  .then(data => {
    if (data.error) {
      alert('Error creating dashboard: ' + data.error);
    } else {
      alert('Dashboard created successfully! URL: ' + data.url);
      location.reload(); // Refresh to show new dashboard
    }
  });
}

function deleteDashboard(dashboardUid) {
  if (!confirm('Are you sure you want to delete this dashboard?')) {
    return;
  }
  
  fetch(`/api/dashboards/${dashboardUid}`, {
    method: 'DELETE',
    headers: {
      'X-CSRFToken': getCSRFToken()
    }
  })
  .then(response => response.json())
  .then(data => {
    if (data.error) {
      alert('Error deleting dashboard: ' + data.error);
    } else {
      alert('Dashboard deleted successfully!');
      location.reload();
    }
  });
}

function updateRetention(dashboardUid) {
  const retentionDays = prompt('Enter new retention days (0 = no expiry):', "0");
  
  fetch(`/api/dashboards/${dashboardUid}/retention`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
      'X-CSRFToken': getCSRFToken()
    },
    body: JSON.stringify({
      retention_days: parseInt(retentionDays)
    })
  })
  .then(response => response.json())
  .then(data => {
    if (data.error) {
      alert('Error updating retention: ' + data.error);
    } else {
      alert('Retention updated successfully!');
      location.reload();
    }
  });
}
```

### 4.6 Scheduled Cleanup (Optional)

```go
// scheduleDashboardCleanup schedules automatic deletion of a dashboard
func scheduleDashboardCleanup(dashboardUID string, retentionDays int) {
  if retentionDays <= 0 {
    return // No expiry
  }
  
  expiryTime := time.Now().AddDate(0, 0, retentionDays)
  
  // Schedule cleanup job
  go func() {
    time.Sleep(time.Until(expiryTime))
    if err := DeleteDashboard(dashboardUID); err != nil {
      log.Printf("Failed to cleanup expired dashboard %s: %v", dashboardUID, err)
    }
  }()
}

// cancelDashboardCleanup cancels scheduled cleanup for a dashboard
func cancelDashboardCleanup(dashboardUID string) {
  // Implementation depends on your cleanup scheduling mechanism
  // Could use a map of scheduled cleanups, or a proper job scheduler
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

### 4.7 Dashboard Cleanup (Optional)

Implement cleanup for old dashboards (only if user specified retention):

```go
func CleanupExpiredDashboards() error {
  // Find dashboards that have passed their expiry time
  // Delete them via API
  // Log cleanup actions
  return nil
}
```

## 5. Security Considerations

### 5.1 Authentication Strategy
- **Admin Password**: Used for initial setup and token creation
- **API Token**: Used for dashboard operations (more secure for automation)
- **Anonymous Access**: Limited to "Public Tests" folder only

### 5.2 Data Isolation
- All dashboard queries filter by `testid` tag
- Anonymous users cannot access other dashboards or data
- Template variables are hidden to prevent manipulation

### 5.3 Token Management
- Store API tokens securely (Kubernetes secrets)
- Rotate tokens periodically
- Use minimal required permissions (Editor role)

### 5.4 Retention Management
- Dashboards live forever by default (no automatic expiry)
- User controls retention periods
- Manual deletion capabilities
- Optional scheduled cleanup for user-specified retention

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
  - name: GRAFANA_DASHBOARD_LIVE_REFRESH
    value: "5s"
  - name: GRAFANA_DASHBOARD_RESULTS_REFRESH
    value: "1m"
```

## Acceptance Criteria

* **User Choice**: Users can choose whether to create live and/or results dashboards via webapp
* **Manual Creation**: Users can create dashboards for existing completed tests via webapp
* **Dashboard Management**: Users can list, delete, and update retention for dashboards via webapp
* **Data Isolation**: Panels only display data where `testid == <id>` (verified by existing template)
* **Anonymous Access**: Users can view dashboards without credentials
* **No Default Expiry**: Dashboards persist indefinitely unless user specifies retention
* **URL Generation**: Orderly Ape outputs valid links for created dashboards:

```
Watch test live: https://grafana.example.com/d/live-test-12345?orgId=1
View final results: https://grafana.example.com/d/test-12345?orgId=1
```

* **Non-blocking**: Failed dashboard creation doesn't break the test run
* **Webapp Integration**: Full dashboard management via orderly-ape admin webapp

## Testing Strategy

### 7.1 Template Validation
- Verify all queries in template use `testid` filtering
- Test with sample data to ensure isolation works
- Validate template JSON structure

### 7.2 Authentication Testing
- Test API token creation and usage
- Verify admin password still works for UI access
- Test anonymous access to Public Tests folder

### 7.3 User Choice Testing
- Test dashboard creation with different user options via webapp
- Verify manual dashboard creation for completed tests via webapp
- Test dashboard listing and management functions via webapp

### 7.4 Retention Testing
- Test dashboard creation with and without retention periods
- Verify scheduled cleanup works correctly
- Test manual retention updates via webapp

### 7.5 Integration Testing
- Run complete test workflow with user choices via webapp
- Verify URL generation and accessibility
- Test data isolation with multiple concurrent tests

### 7.6 Webapp UI Testing
- Test dashboard options on test launch page
- Verify dashboard management interface on test details page
- Test API endpoints for dashboard operations
- Verify JavaScript functions for dashboard management

## Use Cases

### 7.1 Test Execution Monitoring
- **Optional Live Monitoring**: Users choose whether to create live dashboards via webapp
- **Stakeholder Visibility**: Share live URLs with stakeholders during execution
- **Real-time Feedback**: Monitor system performance as tests run

### 7.2 Post-Test Analysis
- **Optional Results**: Users choose whether to create results dashboards via webapp
- **Manual Creation**: Create dashboards for historical tests via webapp
- **Permanent Records**: Keep important test results indefinitely

### 7.3 Dashboard Management
- **List Dashboards**: See all dashboards for a specific test via webapp
- **Delete Dashboards**: Remove dashboards when no longer needed via webapp
- **Update Retention**: Change expiry times for existing dashboards via webapp

## Webapp Features

### 7.1 Test Launch Page
- Dashboard creation options with checkboxes and retention inputs
- Clear labeling for live vs results dashboard options
- Default values (no expiry) for retention periods

### 7.2 Test Details Page
- Dashboard section showing existing dashboards
- Create dashboard buttons for live and results types
- Dashboard information display (title, URL, creation time, expiry)
- Action buttons for update retention and delete

### 7.3 Dashboard Management Page
- Comprehensive dashboard listing across all tests
- Filtering and search capabilities
- Bulk operations for multiple dashboards
- Pagination for large numbers of dashboards

### 7.4 API Endpoints
- RESTful API for all dashboard operations
- Proper error handling and status codes
- CSRF protection for webapp integration
- JSON responses for frontend consumption

## References

* [Grafana HTTP API (Dashboards)](https://grafana.com/docs/grafana/latest/http_api/dashboard/#create-or-update-dashboard)
* [Grafana HTTP API (Folders & Permissions)](https://grafana.com/docs/grafana/latest/http_api/folder/)
* [Grafana HTTP API (API Keys)](https://grafana.com/docs/grafana/latest/http_api/auth/)
* [Hide template variables in JSON: Set `"hide": 2`](https://grafana.com/docs/grafana/latest/dashboards/variables/add-template-variables/#variable-options)
* [Grafana Dashboard Refresh Settings](https://grafana.com/docs/grafana/latest/dashboards/manage-dashboards/#dashboard-settings)
* Existing template: `grafana/test-results.json` (already implements proper `testid` filtering)