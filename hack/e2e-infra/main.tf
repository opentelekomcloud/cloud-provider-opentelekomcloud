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

# Disposable VPC + subnet, created only when no existing network_id is
# supplied. CIDRs sit inside the default var.vpc_cidr (10.0.0.0/8) so the
# security group rules below cover them.
resource "opentelekomcloud_vpc_v1" "vpc" {
  count = var.network_id == "" ? 1 : 0
  name  = "${var.prefix}-vpc"
  cidr  = "10.36.0.0/16"
}

resource "opentelekomcloud_vpc_subnet_v1" "subnet" {
  count         = var.network_id == "" ? 1 : 0
  name          = "${var.prefix}-subnet"
  vpc_id        = opentelekomcloud_vpc_v1.vpc[0].id
  cidr          = "10.36.0.0/24"
  gateway_ip    = "10.36.0.1"
  primary_dns   = "100.125.4.25"
  secondary_dns = "100.125.129.199"
}

locals {
  # OTC quirk: a VPC subnet's ID is the neutron *network* UUID, while the
  # neutron subnet UUID is exposed as its subnet_id attribute.
  network_id = var.network_id != "" ? var.network_id : opentelekomcloud_vpc_subnet_v1.subnet[0].id
  subnet_id  = var.subnet_id != "" ? var.subnet_id : opentelekomcloud_vpc_subnet_v1.subnet[0].subnet_id
  vpc_id     = var.vpc_id != "" ? var.vpc_id : (var.network_id == "" ? opentelekomcloud_vpc_v1.vpc[0].id : "")
}

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
    uuid = local.network_id
  }
}

data "opentelekomcloud_networking_port_v2" "node" {
  device_id = opentelekomcloud_compute_instance_v2.node.id
}

resource "opentelekomcloud_networking_floatingip_associate_v2" "node" {
  floating_ip = opentelekomcloud_networking_floatingip_v2.node.address
  port_id     = data.opentelekomcloud_networking_port_v2.node.id
}
