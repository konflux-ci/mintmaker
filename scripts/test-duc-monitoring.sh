#!/bin/bash

# Test script for DependencyUpdateCheck monitoring setup
# This script validates that the Prometheus rules and configuration are correct

set -e

PROMETHEUS_DIR="prometheus-3.2.1.linux-amd64"
PROMTOOL="${PROMETHEUS_DIR}/promtool"

echo "🔍 Testing DependencyUpdateCheck monitoring setup..."

# Check if promtool exists
if [ ! -f "$PROMTOOL" ]; then
    echo "❌ promtool not found at $PROMTOOL"
    echo "Please ensure Prometheus is downloaded in the project directory"
    exit 1
fi

echo "✅ Found promtool"

# Test 1: Validate Prometheus configuration
echo "📝 Validating Prometheus configuration..."
if $PROMTOOL check config "${PROMETHEUS_DIR}/prometheus.yml"; then
    echo "✅ Prometheus configuration is valid"
else
    echo "❌ Prometheus configuration has errors"
    exit 1
fi

# Test 2: Validate recording rules
echo "📊 Validating DUC monitoring recording rules..."
if $PROMTOOL check rules "${PROMETHEUS_DIR}/mintmaker_duc_monitoring_rules.yml"; then
    echo "✅ DUC monitoring recording rules are valid"
else
    echo "❌ DUC monitoring recording rules have errors"
    exit 1
fi

# Test 3: Validate alert rules
echo "🚨 Validating DUC alert rules..."
if $PROMTOOL check rules "${PROMETHEUS_DIR}/mintmaker_duc_alerts.yaml"; then
    echo "✅ DUC alert rules are valid"
else
    echo "❌ DUC alert rules have errors"
    exit 1
fi

# Test 4: Check if Go code compiles
echo "🔨 Checking if Go code compiles..."
if go build -o /tmp/test-mintmaker ./cmd/manager/; then
    echo "✅ Go code compiles successfully"
    rm -f /tmp/test-mintmaker
else
    echo "❌ Go code compilation failed"
    exit 1
fi

echo ""
echo "🎉 All tests passed! DependencyUpdateCheck monitoring setup is ready."
echo ""
echo "Next steps:"
echo "1. Deploy the updated controller with metrics"
echo "2. Configure Prometheus to scrape the controller metrics"
echo "3. Import the recording and alert rules into your Prometheus/OpenShift monitoring"
echo "4. Test the alerts by waiting for the 4-hour window"
echo ""
echo "For detailed instructions, see: docs/dependency-update-check-monitoring.md"