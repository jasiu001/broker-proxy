#!/usr/bin/env bash

set -o errexit

SM_USER=$1
SM_PASSWORD=$2
URL=$3

SECRET_NAME="service-manager-credentials"
CLUSTER_ADDON_NAME="broker-proxy-k8s-addon"
readonly CURRENT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

if [[ -z $SM_USER || -z $SM_PASSWORD || -z URL ]];then
    echo "User and Password to ServiceManager is required"
    exit 0
fi

echo "Create Secret"
kubectl create secret \
    generic $SECRET_NAME \
    --from-literal=username=$SM_USER \
    --from-literal=password=$SM_PASSWORD

if [[ $(kubectl get clusteraddonsconfigurations $CLUSTER_ADDON_NAME -ojson | jq -r .metadata.name) != "$CLUSTER_ADDON_NAME" ]]; then
  echo "Create ClusterAddonConfiguration"
  kubectl apply -f $CURRENT_DIR/broker-proxy-k8s-cluster-addon.yaml
else
  echo "ClusterAddonConfiguration exist"
fi

LIMIT=120
COUNTER=0

function checkAddonStatus() {
  STATUS=$(kubectl get clusteraddonsconfigurations $CLUSTER_ADDON_NAME -ojson | jq -r '.status.repositories[0].status')
  if [[ $STATUS == "Ready" ]]; then
    echo "ClusterAddonConfiguration is ready"
    return 0
  else
    echo "ClusterAddonConfiguration is not ready (current status: $STATUS)"
    return 1
  fi
}

while [ ${COUNTER} -lt ${LIMIT} ]; do
  if checkAddonStatus $1; then
    COUNTER=$LIMIT
  else
    (( COUNTER++ ))
    sleep 1
  fi
done

echo "Create ServiceInstance"
cat <<EOF | kubectl apply -f -
apiVersion: servicecatalog.k8s.io/v1beta1
kind: ServiceInstance
metadata:
  name: service-broker-proxy-k8s
spec:
  clusterServiceClassExternalName: service-broker-proxy-k8s
  clusterServicePlanExternalName: default
  parameters:
    config:
      sm:
        url: $URL
    secretName: $SECRET_NAME
EOF

LIMIT=120
COUNTER=0

function checkInstanceStatus() {
  STATUS=$(kubectl get serviceinstances.servicecatalog.k8s.io service-broker-proxy-k8s -ojson | jq -r '.status.conditions | .[] | "\(.status)-\(.type)"')
  if [[ $STATUS == "True-Ready" ]]; then
    echo "ServiceInstance is ready"
    return 0
  else
    echo "ServiceInstance is not ready (current status: $STATUS)"
    return 1
  fi
}

while [ ${COUNTER} -lt ${LIMIT} ]; do
  if checkInstanceStatus $1; then
    COUNTER=$LIMIT
  else
    (( COUNTER++ ))
    sleep 1
  fi
done
