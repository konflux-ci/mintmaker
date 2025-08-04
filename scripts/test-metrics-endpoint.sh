#!/bin/bash

# Test script for checking DependencyUpdateCheck metrics in Minikube
set -e

echo "🔍 Testing MintMaker metrics endpoint in Minikube..."

# Check if kubectl is configured for minikube
# if ! kubectl cluster-info | grep -q "minikube"; then
#     echo "❌ Not connected to minikube cluster"
#     echo "Run: minikube start && kubectl config use-context minikube"
#     exit 1
# fi

# Check if controller is running
echo "📋 Checking controller deployment..."
if ! kubectl get deployment -n mintmaker-system mintmaker-controller-manager >/dev/null 2>&1; then
    echo "❌ MintMaker controller not deployed"
    echo "Run: make deploy IMG=mintmaker:local"
    exit 1
fi

echo "✅ Controller found"

# Get pod name
POD_NAME=$(kubectl get pods -n mintmaker-system -l control-plane=controller-manager -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)

if [ -z "$POD_NAME" ]; then
    echo "❌ No controller pod found"
    exit 1
fi

echo "📦 Controller pod: $POD_NAME"

# Check if pod is ready
POD_STATUS=$(kubectl get pod -n mintmaker-system $POD_NAME -o jsonpath='{.status.phase}')
if [ "$POD_STATUS" != "Running" ]; then
    echo "❌ Pod not running (status: $POD_STATUS)"
    exit 1
fi

echo "✅ Pod is running"

# Test metrics endpoint
echo "🌐 Testing metrics endpoint..."

# Method 1: Direct pod exec
echo "📊 Fetching all metrics..."
if kubectl exec -n mintmaker-system $POD_NAME -- curl -s http://localhost:8080/metrics >/dev/null; then
    echo "✅ Metrics endpoint accessible"
else
    echo "❌ Cannot access metrics endpoint"
    exit 1
fi

# Check for DependencyUpdateCheck metrics
echo "🔍 Looking for DependencyUpdateCheck metrics..."
DUC_METRICS=$(kubectl exec -n mintmaker-system $POD_NAME -- curl -s http://localhost:8080/metrics | grep mintmaker_dependency_update_check || true)

if [ -n "$DUC_METRICS" ]; then
    echo "✅ Found DependencyUpdateCheck metrics:"
    echo "$DUC_METRICS"
else
    echo "⚠️  No DependencyUpdateCheck metrics found yet"
    echo "This is normal if no DUC resources have been created"
fi

# Check for other mintmaker metrics
echo "📈 All MintMaker metrics:"
kubectl exec -n mintmaker-system $POD_NAME -- curl -s http://localhost:8080/metrics | grep mintmaker || echo "No mintmaker metrics found"

echo ""
echo "🎉 Metrics endpoint test completed!"
echo ""
echo "💡 To manually test:"
echo "   kubectl port-forward -n mintmaker-system deployment/mintmaker-controller-manager 8080:8080"
echo "   curl http://localhost:8080/metrics | grep mintmaker_dependency_update_check"
echo ""
echo "🧪 To create a test DependencyUpdateCheck:"
echo "   kubectl apply -f config/samples/appstudio_v1alpha1_dependencyupdatecheck.yaml"