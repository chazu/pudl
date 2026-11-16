#!/bin/sh
# Prints the cluster of kubectl's current context as one JSON object for
# k8s-cluster: the context and cluster entries of the minified kubeconfig
# (no credentials: config view omits certificate data and tokens, and the
# user entry is not printed), the output of kubectl version, and host, the
# API server's host (scheme, a :443 port and a trailing slash removed). A
# server that cannot be reached fails the run: kubectl version exits 1.
# Usage: cluster.sh
set -eu
context=$(kubectl config view --minify -o jsonpath='{.contexts[0]}')
cluster=$(kubectl config view --minify -o jsonpath='{.clusters[0]}')
server=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}')
version=$(kubectl version -o json)
host=${server#https://}
host=${host#http://}
host=${host%/}
host=${host%:443}
printf '{"context":%s,"cluster":%s,"version":%s,"host":"%s"}\n' "$context" "$cluster" "$version" "$host"
