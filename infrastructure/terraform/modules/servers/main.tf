terraform {
  required_providers {
    hcloud = {
      source  = "hetznercloud/hcloud"
      version = "~> 1.45"
    }
  }
}

resource "hcloud_ssh_key" "main" {
  name       = "${var.cluster_name}-key"
  public_key = var.ssh_public_key
}

resource "hcloud_server" "master" {
  name         = "${var.cluster_name}-master"
  server_type  = var.master_server_type
  image        = var.image
  location     = var.location
  ssh_keys     = [hcloud_ssh_key.main.id]
  firewall_ids = var.firewall_ids

  labels = {
    cluster = var.cluster_name
    role    = "master"
  }

  user_data = templatefile("${path.module}/cloud-init.yaml", {
    hostname = "${var.cluster_name}-master"
  })

  network {
    network_id = var.network_id
    ip         = var.master_private_ip
  }

  public_net {
    ipv4_enabled = true
    ipv6_enabled = true
  }

  lifecycle {
    ignore_changes = [user_data]
  }
}

resource "hcloud_server" "worker" {
  count        = var.worker_count
  name         = "${var.cluster_name}-worker-${count.index + 1}"
  server_type  = var.worker_server_type
  image        = var.image
  location     = var.location
  ssh_keys     = [hcloud_ssh_key.main.id]
  firewall_ids = var.firewall_ids

  labels = {
    cluster = var.cluster_name
    role    = "worker"
    index   = count.index + 1
  }

  user_data = templatefile("${path.module}/cloud-init.yaml", {
    hostname = "${var.cluster_name}-worker-${count.index + 1}"
  })

  network {
    network_id = var.network_id
    ip         = cidrhost(var.worker_ip_range, count.index + 1)
  }

  public_net {
    ipv4_enabled = true
    ipv6_enabled = true
  }

  lifecycle {
    ignore_changes = [user_data]
  }
}
