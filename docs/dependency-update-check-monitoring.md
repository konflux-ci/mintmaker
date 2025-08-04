# DependencyUpdateCheck Resource Monitoring

This document describes the monitoring setup for DependencyUpdateCheck (DUC) resources to ensure they are created within the expected 4-hour window.

## Overview

The monitoring solution tracks DependencyUpdateCheck resource creation and alerts when:
- DUC creation metrics are unavailable
- DUC creation is approaching the SLA limit (warning at 4 hours)
- No DUC has been created within 4.5 hours (4 hours + 30 minutes tolerance)

## Components

### 1. Metrics Collection

**New Metrics Added:**
- `mintmaker_dependency_update_check_creation_time`: Unix timestamp when DUC resources are created

**Location:** `internal/pkg/metrics/common.go`

### 2. Controller Integration

The DependencyUpdateCheck controller now emits metrics when processing new DUC resources.

**Location:** `internal/controller/dependencyupdatecheck_controller.go`
**Function:** `RecordDependencyUpdateCheckCreation()` is called in the `Reconcile()` method

### 3. Prometheus Recording Rules

**File:** `prometheus-3.2.1.linux-amd64/mintmaker_duc_monitoring_rules.yml`

**Rules:**
- `mintmaker_dependency_update_check_latest_creation_time`: Latest DUC creation time
- `mintmaker_dependency_update_check_time_since_last_creation_seconds`: Time since last DUC (seconds)
- `mintmaker_dependency_update_check_time_since_last_creation_hours`: Time since last DUC (hours)
- `mintmaker_dependency_update_check_creation_overdue`: Boolean indicating if DUC is overdue

### 4. Alerting Rules

**File:** `prometheus-3.2.1.linux-amd64/mintmaker_duc_alerts.yaml`

**Alerts:**
- `MintMakerDependencyUpdateCheckMissing` (Critical): No DUC created within 4.5 hours
- `MintMakerDependencyUpdateCheckMetricsUnavailable` (Warning): Metrics unavailable for 30+ minutes
- `MintMakerDependencyUpdateCheckOverdue` (Warning): No DUC created for 4+ hours

## Configuration

### Prometheus Configuration

The Prometheus configuration (`prometheus.yml`) has been updated to include:
- New rule files for DUC monitoring
- Scrape configuration for the MintMaker controller metrics endpoint

### Expected Timeline

```
0h ---------- 4h ---------- 4.5h ---------->
   │          │             │
   │          │             └── CRITICAL Alert: DUC Missing
   │          └── WARNING Alert: DUC Overdue  
   └── DUC should be created every 4 hours
```

## Testing the Setup

### 1. Verify Metrics are Exposed

```bash
# Check if the controller is exposing metrics
curl http://localhost:8080/metrics | grep mintmaker_dependency_update_check

# Expected output:
# mintmaker_dependency_update_check_creation_time{namespace="...",name="..."} 1703123456
```

### 2. Verify Prometheus Rules

```bash
# Check if Prometheus can load the rules
./promtool check rules mintmaker_duc_monitoring_rules.yml
./promtool check rules mintmaker_duc_alerts.yaml

# Check Prometheus config
./promtool check config prometheus.yml
```

### 3. Query Recording Rules

In Prometheus UI or via API:

```promql
# Check latest DUC creation time
mintmaker_dependency_update_check_latest_creation_time

# Check time since last DUC creation (in hours)
mintmaker_dependency_update_check_time_since_last_creation_hours

# Check if DUC creation is overdue
mintmaker_dependency_update_check_creation_overdue
```

### 4. Test Alert Conditions

To test the alerting:

1. **Wait for 4+ hours without creating a DUC** - Should trigger warning alert
2. **Wait for 4.5+ hours without creating a DUC** - Should trigger critical alert
3. **Stop the controller** - Should trigger metrics unavailable alert after 30 minutes

## Integration with Existing Infrastructure

### Kubernetes/OpenShift Deployment

If deploying to Kubernetes/OpenShift, ensure:

1. **ServiceMonitor Configuration:**
   ```yaml
   apiVersion: monitoring.coreos.com/v1
   kind: ServiceMonitor
   metadata:
     name: mintmaker-controller
   spec:
     selector:
       matchLabels:
         app: mintmaker-controller
     endpoints:
     - port: metrics
       interval: 30s
       path: /metrics
   ```

2. **PrometheusRule Resources:**
   - The YAML files are already in the correct format for OpenShift/Kubernetes
   - Apply them using `kubectl apply -f` or include in your manifests

### Alertmanager Integration

Configure Alertmanager to route alerts to appropriate channels:

```yaml
# alertmanager.yml
route:
  group_by: ['alertname', 'source_cluster']
  routes:
  - match:
      team: mintmaker
    receiver: mintmaker-team
    
receivers:
- name: mintmaker-team
  slack_configs:
  - api_url: 'YOUR_SLACK_WEBHOOK'
    channel: '#mintmaker-alerts'
    title: 'MintMaker Alert: {{ .GroupLabels.alertname }}'
    text: '{{ range .Alerts }}{{ .Annotations.description }}{{ end }}'
```

## Troubleshooting

### Common Issues

1. **Metrics not appearing in Prometheus:**
   - Check if controller is running and metrics endpoint is accessible
   - Verify scrape configuration in prometheus.yml
   - Check Prometheus logs for scrape errors

2. **Rules not loading:**
   - Validate YAML syntax with promtool
   - Check Prometheus logs for rule evaluation errors
   - Ensure rule files are in the correct path

3. **Alerts not firing:**
   - Check if recording rules are working correctly
   - Verify alert rule expressions in Prometheus UI
   - Check Alertmanager configuration and routing

### Manual Testing Commands

```bash
# Build and run the controller locally
make run

# Create a test DependencyUpdateCheck resource
kubectl apply -f config/samples/appstudio_v1alpha1_dependencyupdatecheck.yaml

# Check metrics after creation
curl http://localhost:8080/metrics | grep dependency_update_check

# Start Prometheus with the updated configuration
cd prometheus-3.2.1.linux-amd64
./prometheus --config.file=prometheus.yml --storage.tsdb.path=./data
```

## Maintenance

### Regular Tasks

1. **Monitor alert noise:** Adjust thresholds if false positives occur
2. **Review SLA:** Update 4-hour window if business requirements change
3. **Update documentation:** Keep runbooks current with operational procedures

### Configuration Updates

If you need to modify the timing:
- **4-hour window:** Update recording rules and alert expressions
- **30-minute tolerance:** Modify the `4.5` threshold in recording rules and alerts
- **Alert severity:** Adjust labels in alert rule files