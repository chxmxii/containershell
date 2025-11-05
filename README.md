Quickly exec into any contianer (podman,docker, crio etc...)

usage:


config:
[defaults]
default_engine = crio

[configs]
kubeconfig_path = <path_to_kubeconfig>


cs:

cs -e podman <container_name_or_id> <shell> 

cs debug <contaier_name> // this will mount

cs env <container_name> // get env from container

cs logs <container_name>