output "master_public_ip" {
  description = "Public IP of K3s master"
  value       = module.servers.master_public_ip
}

output "master_private_ip" {
  description = "Private IP of K3s master"
  value       = module.servers.master_private_ip
}

output "worker_public_ips" {
  description = "Public IPs of K3s workers"
  value       = module.servers.worker_public_ips
}

output "worker_private_ips" {
  description = "Private IPs of K3s workers"
  value       = module.servers.worker_private_ips
}

output "load_balancer_ip" {
  description = "Public IP of the load balancer"
  value       = module.load_balancer.load_balancer_ipv4
}

output "domain" {
  description = "Domain name"
  value       = var.domain
}

output "dns_records" {
  description = "DNS records to create"
  value = {
    "A ${var.domain}"         = module.load_balancer.load_balancer_ipv4
    "A api.${var.domain}"     = module.load_balancer.load_balancer_ipv4
    "A grafana.${var.domain}" = module.servers.master_public_ip
  }
}

output "ssh_master" {
  description = "SSH command for master"
  value       = "ssh root@${module.servers.master_public_ip}"
}

output "ssh_workers" {
  description = "SSH commands for workers"
  value       = [for ip in module.servers.worker_public_ips : "ssh root@${ip}"]
}

output "volumes" {
  description = "Volume information"
  value = {
    postgres = module.volumes.postgres_volume_path
    redis    = module.volumes.redis_volume_path
    minio    = module.volumes.minio_volume_path
  }
}
