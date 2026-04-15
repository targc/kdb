bash ./scripts/setup-cluster.sh
bash ./scripts/setup-trafik-daemonset.sh
bash ./scripts/setup-operator.sh

./scripts/create-tls-cert.sh tmp/tls.crt tmp/tls.key
./scripts/check-cert.sh tmp/tls-cert.secret.yaml

kubectl apply -f examples/crds/
