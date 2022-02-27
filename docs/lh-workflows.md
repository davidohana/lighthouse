# Lighthouse Agent Workflows

## Processes

1. `serviceExportSyncer` - Create a local service import in `submariner-operator` NS for each (local) service export.

1. `serviceImportController` - Create Endpoint controller for each local service import. Endpoint controller creates an endpoint slice for each endpoints object.

1. `serviceImportSyncer` - Bidirectional, uploads local service imports to broker + downloads remote service imports from broker to `submariner-operator` NS.

1. `endpointSliceSyncer` - Bidirectional, uploads local endpoint slices to broker + downloads remote endpoint slices from broker to service NS. Syncs only EPS that are managed by submariner.

1. `serviceSyncer` - watches deletion of services and update corresponding service exports to invalid.

## Workflow - Export Service

Tested on two submariner-connected clusters, `cluster1` and `cluster1` created with `kind`. `cluster1` is also the broker. 

1. Deployment `monti-host` is created on cluster1 in NS `monti`


2. Pods `monti1`, `monti2` are created on cluster1 NS `monti`

```
-> % kubectl get pods -n monti
NAME                                       READY   STATUS    RESTARTS   AGE
monti-deployment-server-6fb466fbfd-8s6t6   1/1     Running   0          4m51s
monti-deployment-server-6fb466fbfd-lggjb   1/1     Running   0          4m51s
```

3. service `monti-host` is created on cluster1 NS= `monti`

```
-> % kubectl get service -n monti
NAME         TYPE        CLUSTER-IP    EXTERNAL-IP   PORT(S)    AGE
monti-host   ClusterIP   100.0.68.66   <none>        8091/TCP   7m56s
```

```yaml
-> % kubectl get services -n monti -o yaml
apiVersion: v1
items:
- apiVersion: v1
  kind: Service
  metadata:
    annotations:
      kubectl.kubernetes.io/last-applied-configuration: |
        {"apiVersion":"v1","kind":"Service","metadata":{"annotations":{},"name":"monti-host","namespace":"monti"},"spec":{"ports":[{"port":8091,"protocol":"TCP","targetPort":8090}],"selector":{"app":"monti-server"}}}
    creationTimestamp: "2022-02-16T14:52:33Z"
    name: monti-host
    namespace: monti
    resourceVersion: "4063"
    uid: 8ee132d1-1857-4b3e-9780-c6e10e1702d1
  spec:
    clusterIP: 100.0.103.24
    clusterIPs:
    - 100.0.103.24
    ipFamilies:
    - IPv4
    ipFamilyPolicy: SingleStack
    ports:
    - port: 8091
      protocol: TCP
      targetPort: 8090
    selector:
      app: monti-server
    sessionAffinity: None
    type: ClusterIP
  status:
    loadBalancer: {}
kind: List
metadata:
  resourceVersion: ""
  selfLink: ""
```

4. Endpoints and Endpoint Slice is created by k8s for service on cluster1 NS `monti`

```
-> % kubectl get endpoints --context cluster1 -n monti
NAME         ENDPOINTS                      AGE
monti-host   10.0.0.10:8090,10.0.0.9:8090   24h
```

```yaml
-> % kubectl get endpoints --context cluster1 -n monti -o yaml
apiVersion: v1
items:
- apiVersion: v1
  kind: Endpoints
  metadata:
    annotations:
      endpoints.kubernetes.io/last-change-trigger-time: "2022-02-14T14:14:37Z"
    creationTimestamp: "2022-02-14T14:14:35Z"
    name: monti-host
    namespace: monti
    resourceVersion: "2378"
    uid: 16abf07c-eede-490c-8b80-6300a6260aeb
  subsets:
  - addresses:
    - ip: 10.0.0.10
      nodeName: cluster1-worker
      targetRef:
        kind: Pod
        name: monti-deployment-server-6fb466fbfd-lggjb
        namespace: monti
        resourceVersion: "2373"
        uid: 6c032a59-1a59-446a-80d2-f8694adc9a85
    - ip: 10.0.0.9
      nodeName: cluster1-worker
      targetRef:
        kind: Pod
        name: monti-deployment-server-6fb466fbfd-8s6t6
        namespace: monti
        resourceVersion: "2370"
        uid: 7941642a-d487-450b-aabf-5134ad0bc624
    ports:
    - port: 8090
      protocol: TCP
kind: List
metadata:
  resourceVersion: ""
  selfLink: ""
```

```
-> % kubectl get endpointslice -A --context cluster1
NAMESPACE             NAME                                          ADDRESSTYPE   PORTS        ENDPOINTS            AGE
default               kubernetes                                    IPv4          6443         172.18.0.3           9m16s
kube-system           kube-dns-cspwc                                IPv4          53,9153,53   10.0.0.2,10.0.0.4    9m1s
monti                 monti-host-kfd2z                              IPv4          8090         10.0.0.9,10.0.0.10   15s
submariner-operator   submariner-gateway-metrics-c7fm8              IPv4          8080         172.18.0.6           7m2s
submariner-operator   submariner-globalnet-metrics-ldcjk            IPv4          8081         172.18.0.6           7m1s
submariner-operator   submariner-lighthouse-agent-metrics-kbzzw     IPv4          8082         10.0.0.6             6m59s
submariner-operator   submariner-lighthouse-coredns-hdkwm           IPv4          53           10.0.0.7,10.0.0.8    6m56s
submariner-operator   submariner-lighthouse-coredns-metrics-g8dls   IPv4          9153         10.0.0.7,10.0.0.8    6m56s
submariner-operator   submariner-operator-metrics-kfmbx             IPv4          8686,8383    10.0.0.5             7m21s
```

5. service `monti-host` is exported

```
-> % ./submariner-operator/bin/subctl  export service --namespace monti monti-host
Service exported successfully

-> % kubectl get serviceexport -A --context cluster1
NAMESPACE   NAME         AGE
monti       monti-host   2m39s
```

```yaml
-> % kubectl get serviceexport -A --context cluster1 -o yaml
apiVersion: v1
items:
- apiVersion: multicluster.x-k8s.io/v1alpha1
  kind: ServiceExport
  metadata:
    creationTimestamp: "2022-02-14T14:20:50Z"
    generation: 1
    name: monti-host
    namespace: monti
    resourceVersion: "3389"
    uid: 433c14ed-d978-463d-b44d-906cdc0411c3
  status:
    conditions:
    - lastTransitionTime: "2022-02-14T14:20:50Z"
      message: Service doesn't have a global IP yet
      reason: ServiceGlobalIPUnavailable
      status: "False"
      type: Valid
    - lastTransitionTime: "2022-02-14T14:20:50Z"
      message: Awaiting sync of the ServiceImport to the broker
      reason: AwaitingSync
      status: "False"
      type: Valid
    - lastTransitionTime: "2022-02-14T14:20:50Z"
      message: Service was successfully synced to the broker
      reason: ""
      status: "True"
      type: Valid
kind: List
metadata:
  resourceVersion: ""
  selfLink: ""
```

6. LH Agent `serviceExportSyncer` is notified for new service export and creates a service import on `submariner-operator` NS on cluster1

```yaml
-> % kubectl get serviceimport/monti-host-monti-cluster1 -n submariner-operator --context cluster1 -o yaml
apiVersion: multicluster.x-k8s.io/v1alpha1
kind: ServiceImport
metadata:
  annotations:
    cluster-ip: 242.254.1.253
    origin-name: monti-host
    origin-namespace: monti
  creationTimestamp: "2022-02-14T14:20:50Z"
  generation: 1
  labels:
    lighthouse.submariner.io/sourceCluster: cluster1
    lighthouse.submariner.io/sourceName: monti-host
    lighthouse.submariner.io/sourceNamespace: monti
  name: monti-host-monti-cluster1
  namespace: submariner-operator
  resourceVersion: "3388"
  uid: 058a4936-d0d3-4213-b024-e5b8ce4c4e8e
spec:
  ips:
  - 242.254.1.253
  ports:
  - port: 8091
    protocol: TCP
  sessionAffinityConfig: {}
  type: ClusterSetIP
```

7. LH Agent (`serviceImportController`) on cluster1 is notified for the new service import and creates a named `endpointslice` on `monti` ns

```
-> % kubectl get endpointslice -n monti --context cluster1
NAME                  ADDRESSTYPE   PORTS   ENDPOINTS            AGE
monti-host-cluster1   IPv4          8090    10.0.0.10,10.0.0.9   28m
monti-host-kfd2z      IPv4          8090    10.0.0.9,10.0.0.10   35m
```

```yaml
-> % kubectl get endpointslice/monti-host-cluster1 -n monti --context cluster1 -o yaml
addressType: IPv4
apiVersion: discovery.k8s.io/v1beta1
endpoints:
- addresses:
  - 10.0.0.10
  conditions:
    ready: true
  hostname: monti-deployment-server-6fb466fbfd-lggjb
  topology:
    kubernetes.io/hostname: cluster1-worker
- addresses:
  - 10.0.0.9
  conditions:
    ready: true
  hostname: monti-deployment-server-6fb466fbfd-8s6t6
  topology:
    kubernetes.io/hostname: cluster1-worker
kind: EndpointSlice
metadata:
  creationTimestamp: "2022-02-14T14:20:51Z"
  generation: 1
  labels:
    endpointslice.kubernetes.io/managed-by: lighthouse-agent.submariner.io
    lighthouse.submariner.io/sourceCluster: cluster1
    lighthouse.submariner.io/sourceName: monti-host
    lighthouse.submariner.io/sourceNamespace: monti
  name: monti-host-cluster1
  namespace: monti
  resourceVersion: "3393"
  uid: 0bddf537-8c60-4b50-add2-679d05e42a0f
ports:
- name: ""
  port: 8090
  protocol: TCP
```

8. LH Agent (`serviceImportSyncer`) on cluster1 uploads service import to broker NS `submariner-k8s-broker` on cluster1, adding new labels `submariner-io/clusterID` and `submariner-io/originatingNamespace`

```
-> % kubectl get serviceimport -A --context cluster1
NAMESPACE               NAME                        TYPE           IP                  AGE
submariner-k8s-broker   monti-host-monti-cluster1   ClusterSetIP   ["242.254.1.253"]   4m32s
submariner-operator     monti-host-monti-cluster1   ClusterSetIP   ["242.254.1.253"]   4m32s
```

```yaml
-> % kubectl get serviceimport/monti-host-monti-cluster1 -n submariner-k8s-broker --context cluster1 -o yaml
apiVersion: multicluster.x-k8s.io/v1alpha1
kind: ServiceImport
metadata:
  annotations:
    cluster-ip: 242.254.1.253
    origin-name: monti-host
    origin-namespace: monti
  creationTimestamp: "2022-02-14T14:20:50Z"
  generation: 1
  labels:
    lighthouse.submariner.io/sourceCluster: cluster1
    lighthouse.submariner.io/sourceName: monti-host
    lighthouse.submariner.io/sourceNamespace: monti
    submariner-io/clusterID: cluster1
    submariner-io/originatingNamespace: submariner-operator
  name: monti-host-monti-cluster1
  namespace: submariner-k8s-broker
  resourceVersion: "3390"
  uid: 07d1db4f-f2b3-4cfd-95e5-4c16c428b48f
spec:
  ips:
  - 242.254.1.253
  ports:
  - port: 8091
    protocol: TCP
  sessionAffinityConfig: {}
  type: ClusterSetIP
```

9. LH Agent (`endpointSliceSyncer`) on cluster1 uploads endpoints slice to broker

10. LH Agent (`serviceImportSyncer`) on cluster2 downloads service import from broker to NS `submariner-operator`

```bash
-> % kubectl get serviceimport -A --context cluster2
NAMESPACE             NAME                        TYPE           IP                  AGE
submariner-operator   monti-host-monti-cluster1   ClusterSetIP   ["242.254.1.253"]   10m
```

```yaml
-> % kubectl get serviceimport -A --context cluster2 -o yaml
apiVersion: v1
items:
- apiVersion: multicluster.x-k8s.io/v1alpha1
  kind: ServiceImport
  metadata:
    annotations:
      cluster-ip: 242.254.1.253
      origin-name: monti-host
      origin-namespace: monti
    creationTimestamp: "2022-02-14T14:20:50Z"
    generation: 1
    labels:
      lighthouse.submariner.io/sourceCluster: cluster1
      lighthouse.submariner.io/sourceName: monti-host
      lighthouse.submariner.io/sourceNamespace: monti
      submariner-io/clusterID: cluster1
      submariner-io/originatingNamespace: submariner-operator
    name: monti-host-monti-cluster1
    namespace: submariner-operator
    resourceVersion: "3235"
    uid: 334e6cda-ce5a-46a7-b1f8-94839eabc6bb
  spec:
    ips:
    - 242.254.1.253
    ports:
    - port: 8091
      protocol: TCP
    sessionAffinityConfig: {}
    type: ClusterSetIP
kind: List
metadata:
  resourceVersion: ""
  selfLink: ""
```

11. Create NS `monti` on cluster2

12. LH Agent (`endpointSliceSyncer`) on cluster2 downloads endpoint slice from broker to monti ns

```
-> % kubectl get endpointslice -n monti --context cluster2
NAME                  ADDRESSTYPE   PORTS   ENDPOINTS            AGE
monti-host-cluster1   IPv4          8090    10.0.0.10,10.0.0.9   11m
```

```yaml
-> % kubectl get endpointslice/monti-host-cluster1 -n monti --context cluster2 -o yaml
addressType: IPv4
apiVersion: discovery.k8s.io/v1beta1
endpoints:
- addresses:
  - 10.0.0.10
  conditions:
    ready: true
  hostname: monti-deployment-server-6fb466fbfd-lggjb
  topology:
    kubernetes.io/hostname: cluster1-worker
- addresses:
  - 10.0.0.9
  conditions:
    ready: true
  hostname: monti-deployment-server-6fb466fbfd-8s6t6
  topology:
    kubernetes.io/hostname: cluster1-worker
kind: EndpointSlice
metadata:
  creationTimestamp: "2022-02-14T14:35:34Z"
  generation: 1
  labels:
    endpointslice.kubernetes.io/managed-by: lighthouse-agent.submariner.io
    lighthouse.submariner.io/sourceCluster: cluster1
    lighthouse.submariner.io/sourceName: monti-host
    lighthouse.submariner.io/sourceNamespace: monti
    submariner-io/clusterID: cluster1
    submariner-io/originatingNamespace: monti
  name: monti-host-cluster1
  namespace: monti
  resourceVersion: "5653"
  uid: 1eb9548b-90fa-4e03-982d-1cfc770f7e58
ports:
- name: ""
  port: 8090
  protocol: TCP
```

13. `monti-host` service is now accessible remotely from cluster2 to cluster1 at endpoint http://monti-host.monti.svc.clusterset.local:8091.
