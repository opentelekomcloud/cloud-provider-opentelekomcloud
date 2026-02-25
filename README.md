# Kubernetes Cloud Provider for Open Telekom Cloud

The Open Telekom Cloud Controller Manager provides the interface between a Kubernetes cluster and Open Telekom Cloud service APIs.
This project allows a Kubernetes cluster to provision, monitor, and remove Open Telekom Cloud resources necessary for the operation of the cluster.

See [Cloud Controller Manager Administration](https://kubernetes.io/docs/tasks/administer-cluster/running-cloud-controller/)
for more about Kubernetes cloud controller manager.

## Implementation Details

Currently `opentelekomcloud-cloud-controller-manager` is in early development. No controllers are implemented yet.

## Compatibility with Kubernetes

| Kubernetes Version | Latest CCM Version |
|--------------------|--------------------|
| v1.31+             | v0.1.0             |

## More About Cloud Controller Manager

- [Concepts Underlying the Cloud Controller Manager](https://kubernetes.io/docs/concepts/architecture/cloud-controller/)
- [Running cloud controller manager](https://kubernetes.io/docs/tasks/administer-cluster/running-cloud-controller/#running-cloud-controller-manager)
- [Developing Cloud Controller Manager](https://kubernetes.io/docs/tasks/administer-cluster/developing-cloud-controller-manager/)

## Support

Any questions feel free to [submit an issue](https://github.com/opentelekomcloud/cloud-provider-opentelekomcloud/issues/new).

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for details.
