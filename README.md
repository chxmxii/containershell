<div style="display: flex; justify-content: center;">
  <div>
    <h1>Container shell </h1> 
    <p>Quickly exec or debug into any contianer!</p>
  </div>
</div>

--- 

#### Install

```bash
$ git clone https://github.com/chxmxii/containershell.git
$ cd containershell
$ make
```
---

#### Config:
You can configure containershell to set default container engine or default kubeconfig if u intend to use it for k8s containers

```ini
[defaults]
default_engine = crio
kubeconfig_path = <path_to_kubeconfig>
max_tries = 5
default_shell = bash
```
---

#### Usage

```sh
$ cs -e podman <container_name_or_id> <shell> 
$ cs debug <container_name_or_id> // this will mount
$ cs env <container_name_or_id> // get env from container
$ cs logs <container_name_or_id> .. get logs
```
