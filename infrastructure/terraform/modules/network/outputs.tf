output "network_id" {
  description = "ID of the private network"
  value       = hcloud_network.main.id
}

output "subnet_id" {
  description = "ID of the subnet"
  value       = hcloud_network_subnet.main.id
}

output "firewall_id" {
  description = "ID of the firewall"
  value       = hcloud_firewall.main.id
}

output "network_cidr" {
  description = "CIDR of the network"
  value       = hcloud_network.main.ip_range
}
