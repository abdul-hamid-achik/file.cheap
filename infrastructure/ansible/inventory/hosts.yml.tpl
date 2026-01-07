---
all:
  vars:
    ansible_user: root
    ansible_ssh_common_args: '-o StrictHostKeyChecking=no'
    cluster_name: ${cluster_name}
    domain: ${domain}
    k3s_version: v1.30.0+k3s1

  children:
    masters:
      hosts:
        k3s-master:
          ansible_host: ${master_public_ip}
          private_ip: ${master_private_ip}

    workers:
      hosts:
%{ for i, ip in worker_public_ips ~}
        k3s-worker-${i + 1}:
          ansible_host: ${ip}
          private_ip: ${worker_private_ips[i]}
%{ endfor ~}

    k3s_cluster:
      children:
        masters:
        workers:
