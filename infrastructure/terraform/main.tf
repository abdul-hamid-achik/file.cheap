module "network" {
  source = "./modules/network"

  cluster_name    = var.cluster_name
  ssh_allowed_ips = var.ssh_allowed_ips
}

module "servers" {
  source = "./modules/servers"

  cluster_name       = var.cluster_name
  ssh_public_key     = var.ssh_public_key
  master_server_type = var.master_server_type
  worker_server_type = var.worker_server_type
  worker_count       = var.worker_count
  location           = var.location
  network_id         = module.network.network_id
  firewall_ids       = [module.network.firewall_id]

  depends_on = [module.network]
}

module "volumes" {
  source = "./modules/volumes"

  cluster_name      = var.cluster_name
  location          = var.location
  postgres_size     = var.postgres_volume_size
  redis_size        = var.redis_volume_size
  minio_size        = var.minio_volume_size
  storage_server_id = module.servers.worker_ids[0]

  depends_on = [module.servers]
}

module "load_balancer" {
  source = "./modules/load-balancer"

  cluster_name = var.cluster_name
  location     = var.location
  network_id   = module.network.network_id
  worker_ids   = module.servers.worker_ids

  depends_on = [module.servers]
}

resource "local_file" "ansible_inventory" {
  content = templatefile("${path.module}/../ansible/inventory/hosts.yml.tpl", {
    master_public_ip   = module.servers.master_public_ip
    master_private_ip  = module.servers.master_private_ip
    worker_public_ips  = module.servers.worker_public_ips
    worker_private_ips = module.servers.worker_private_ips
    cluster_name       = var.cluster_name
    domain             = var.domain
  })
  filename = "${path.module}/../ansible/inventory/hosts.yml"
}
