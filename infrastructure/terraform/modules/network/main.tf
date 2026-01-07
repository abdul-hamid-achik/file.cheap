terraform {
  required_providers {
    hcloud = {
      source  = "hetznercloud/hcloud"
      version = "~> 1.45"
    }
  }
}

resource "hcloud_network" "main" {
  name     = "${var.cluster_name}-network"
  ip_range = var.network_cidr
}

resource "hcloud_network_subnet" "main" {
  network_id   = hcloud_network.main.id
  type         = "cloud"
  network_zone = var.network_zone
  ip_range     = var.subnet_cidr
}

resource "hcloud_firewall" "main" {
  name = "${var.cluster_name}-firewall"

  rule {
    direction   = "in"
    protocol    = "tcp"
    port        = "22"
    source_ips  = var.ssh_allowed_ips
    description = "SSH"
  }

  rule {
    direction   = "in"
    protocol    = "tcp"
    port        = "80"
    source_ips  = ["0.0.0.0/0", "::/0"]
    description = "HTTP"
  }

  rule {
    direction   = "in"
    protocol    = "tcp"
    port        = "443"
    source_ips  = ["0.0.0.0/0", "::/0"]
    description = "HTTPS"
  }

  rule {
    direction   = "in"
    protocol    = "tcp"
    port        = "6443"
    source_ips  = var.ssh_allowed_ips
    description = "Kubernetes API"
  }

  rule {
    direction   = "in"
    protocol    = "tcp"
    port        = "any"
    source_ips  = [var.network_cidr]
    description = "Internal TCP"
  }

  rule {
    direction   = "in"
    protocol    = "udp"
    port        = "any"
    source_ips  = [var.network_cidr]
    description = "Internal UDP"
  }

  rule {
    direction   = "in"
    protocol    = "icmp"
    source_ips  = [var.network_cidr]
    description = "Internal ICMP"
  }
}
