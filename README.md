# custom-hpa

generate code
```bash
./hack/update-codegen.sh
```

```bash
go build -o custom-hpa-controller .
./custom-hpa-controller -kubeconfig=$HOME/.kube/config
```

## deploy
```bash
kubectl create -f deploy/crd.yaml
kubectl create -f deploy/customhpa.yaml

# delete
kubectl delete -f deploy/customhpa.yaml
kubectl delete -f deploy/crd.yaml
```