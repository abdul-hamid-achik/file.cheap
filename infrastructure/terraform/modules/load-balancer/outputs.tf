output "load_balancer_id" {
  description = "ID of the load balancer"
  value       = hcloud_load_balancer.main.id
}

output "load_balancer_ipv4" {
  description = "Public IPv4 of the load balancer"
  value       = hcloud_load_balancer.main.ipv4
}

output "load_balancer_ipv6" {
  description = "Public IPv6 of the load balancer"
  value       = hcloud_load_balancer.main.ipv6
}

output "load_balancer_private_ip" {
  description = "Private IP of the load balancer"
  value       = var.lb_private_ip
}
