packer {
  required_plugins {
    qemu = {
      version = ">= 1.0.0"
      source  = "github.com/hashicorp/qemu"
    }
  }
}

variable "base_image_url" {
  type    = string
  default = "https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-generic-amd64.qcow2"
}

variable "base_image_checksum" {
  type    = string
  default = "file:https://cloud.debian.org/images/cloud/bookworm/latest/SHA512SUMS"
}

variable "vm_name" {
  type    = string
  default = "osvbng-debian12"
}

variable "dataplane_version" {
  type    = string
  default = env("DATAPLANE_VERSION")
}

variable "accelerator" {
  type    = string
  default = "kvm"
}

source "qemu" "osvbng" {
  iso_url      = var.base_image_url
  iso_checksum = var.base_image_checksum
  disk_image   = true

  output_directory = "output"
  vm_name          = "${var.vm_name}.qcow2"

  disk_size   = "20G"
  format      = "qcow2"
  accelerator = var.accelerator

  memory = 2048
  cpus   = 2

  headless = true

  # Cloud-init seed ISO
  cd_files = ["cloud-init/user-data", "cloud-init/meta-data"]
  cd_label = "cidata"

  ssh_username = "root"
  ssh_password = "osvbng"
  ssh_timeout  = "30m"

  shutdown_command = "shutdown -P now"
}

build {
  sources = ["source.qemu.osvbng"]

  provisioner "shell" {
    inline = ["mkdir -p /tmp/vpp-plugins /tmp/templates"]
  }

  provisioner "file" {
    source      = "../../../bin/osvbngd"
    destination = "/tmp/osvbngd"
  }

  provisioner "file" {
    source      = "../../../bin/osvbngcli"
    destination = "/tmp/osvbngcli"
  }

  provisioner "file" {
    source      = "../../../test-infra/vpp-plugins/"
    destination = "/tmp/vpp-plugins/"
  }

  provisioner "file" {
    source      = "../../../templates/"
    destination = "/tmp/templates/"
  }

  provisioner "shell" {
    environment_vars = [
      "DATAPLANE_VERSION=${var.dataplane_version}",
      "DEBIAN_FRONTEND=noninteractive"
    ]
    scripts = [
      "scripts/install-deps.sh",
      "scripts/install-vpp.sh",
      "scripts/install-frr.sh",
      "scripts/install-osvbng.sh",
      "scripts/configure-services.sh",
      "scripts/cleanup.sh"
    ]
  }

  post-processor "compress" {
    output = "${var.vm_name}.qcow2.gz"
  }
}
