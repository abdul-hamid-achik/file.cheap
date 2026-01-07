output "master_public_ip" {
  description = "Public IP of the master node"
  value       = hcloud_server.master.ipv4_address
}

output "master_private_ip" {
  description = "Private IP of the master node"
  value       = one(hcloud_server.master.network[*].ip)
}

output "master_id" {
  description = "ID of the master server"
  value       = hcloud_server.master.id
}

output "worker_public_ips" {
  description = "Public IPs of worker nodes"
  value       = hcloud_server.worker[*].ipv4_address
}

output "worker_private_ips" {
  description = "Private IPs of worker nodes"
  value       = [for s in hcloud_server.worker : one(s.network[*].ip)]
}

output "worker_ids" {
  description = "IDs of worker servers"
  value       = hcloud_server.worker[*].id
}

output "ssh_key_id" {
  description = "ID of the SSH key"
  value       = hcloud_ssh_key.main.id
}
