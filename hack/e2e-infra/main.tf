terraform {
  required_version = ">= 1.5"
  required_providers {
    opentelekomcloud = {
      source  = "opentelekomcloud/opentelekomcloud"
      version = ">= 1.36.0"
    }
  }
}

# Credentials come from clouds.yaml; select the entry with OS_CLOUD.
provider "opentelekomcloud" {}

resource "opentelekomcloud_networking_floatingip_v2" "node" {
  pool = "admin_external_net"
}

resource "opentelekomcloud_compute_keypair_v2" "node" {
  name       = "${var.prefix}-key"
  public_key = file(pathexpand(var.ssh_public_key_file))
}

resource "opentelekomcloud_networking_secgroup_v2" "node" {
  name        = "${var.prefix}-sg"
  description = "k3s e2e test node for cloud-provider-opentelekomcloud"
}

resource "opentelekomcloud_networking_secgroup_rule_v2" "ssh" {
  direction         = "ingress"
  ethertype         = "IPv4"
  protocol          = "tcp"
  port_range_min    = 22
  port_range_max    = 22
  remote_ip_prefix  = var.admin_cidr
  security_group_id = opentelekomcloud_networking_secgroup_v2.node.id
}

resource "opentelekomcloud_networking_secgroup_rule_v2" "kube_api" {
  direction         = "ingress"
  ethertype         = "IPv4"
  protocol          = "tcp"
  port_range_min    = 6443
  port_range_max    = 6443
  remote_ip_prefix  = var.admin_cidr
  security_group_id = opentelekomcloud_networking_secgroup_v2.node.id
}

# Load balancer health checks and NodePort traffic from inside the VPC.
resource "opentelekomcloud_networking_secgroup_rule_v2" "vpc_tcp" {
  direction         = "ingress"
  ethertype         = "IPv4"
  protocol          = "tcp"
  port_range_min    = 1
  port_range_max    = 65535
  remote_ip_prefix  = var.vpc_cidr
  security_group_id = opentelekomcloud_networking_secgroup_v2.node.id
}

resource "opentelekomcloud_networking_secgroup_rule_v2" "vpc_udp" {
  direction         = "ingress"
  ethertype         = "IPv4"
  protocol          = "udp"
  port_range_min    = 1
  port_range_max    = 65535
  remote_ip_prefix  = var.vpc_cidr
  security_group_id = opentelekomcloud_networking_secgroup_v2.node.id
}

resource "opentelekomcloud_compute_instance_v2" "node" {
  name              = "${var.prefix}-node"
  image_name        = var.image_name
  flavor_name       = var.flavor_name
  key_pair          = opentelekomcloud_compute_keypair_v2.node.name
  availability_zone = var.availability_zone
  security_groups   = [opentelekomcloud_networking_secgroup_v2.node.name]

  user_data = templatefile("${path.module}/cloud-init.tftpl", {
    public_ip   = opentelekomcloud_networking_floatingip_v2.node.address
    k3s_channel = var.k3s_channel
  })

  network {
    uuid = var.network_id
  }
}

resource "opentelekomcloud_compute_floatingip_associate_v2" "node" {
  floating_ip = opentelekomcloud_networking_floatingip_v2.node.address
  instance_id = opentelekomcloud_compute_instance_v2.node.id
}
