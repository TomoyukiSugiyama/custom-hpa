# custom-hpa

generate code
```bash
./hack/update-codegen.sh
```

```bash
go build -o custom-hpa-controller .
./custom-hpa-controller -kubeconfig=$HOME/.kube/config
```

## Develop
To test in local
```bash
go test ./... -covermode=count -coverprofile=c.out
go tool cover -html=c.out
```

## Deploy
```bash
kubectl create -f deploy/deployment.yaml
kubectl create -f deploy/crd.yaml
kubectl create -f deploy/customhpa.yaml

# delete
kubectl delete -f deploy/deployment.yaml
kubectl delete -f deploy/customhpa.yaml
kubectl delete -f deploy/crd.yaml
```