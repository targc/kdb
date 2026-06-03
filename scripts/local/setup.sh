DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

bash "$DIR/setup-cluster.sh"
bash "$DIR/setup-trafik-daemonset.sh"
bash "$DIR/setup-operator.sh"

"$DIR/create-tls-cert.sh" tmp/tls.crt tmp/tls.key
"$DIR/check-cert.sh" tmp/tls-cert.secret.yaml

kubectl apply -f examples/crds/
