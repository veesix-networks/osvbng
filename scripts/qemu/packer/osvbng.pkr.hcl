packer {
  required_plugins {
    qemu = {
      version = ">= 1.0.0"
      source  = "github.com/hashicorp/qemu"
    }
  }
}

variable "os_variant" {
  type    = string
  default = "debian12"
}

variable "iso_url" {
  type = string
  default = "https://cdimage.debian.org/cdimage/archive/12.9.0/amd64/iso-cd/debian-12.9.0-amd64-netinst.iso"
}

variable "iso_checksum" {
  type    = string
  default = "sha256:1257373c706d8c07e6917942736a865dfff557d21d76ea3040bb1039eb72a054"
}

variable "vm_name" {
  type    = string
  default = "osvbng-debian12"
}

variable "vpp_version" {
  type = string
}

variable "frr_version" {
  type = string
}

variable "accelerator" {
  type    = string
  default = "kvm"
}

source "qemu" "osvbng" {
  iso_url      = var.iso_url
  iso_checksum = var.iso_checksum

  output_directory = "output"
  vm_name          = "${var.vm_name}.qcow2"

  disk_size        = "20G"
  format           = "qcow2"
  accelerator      = var.accelerator

  memory           = 2048
  cpus             = 2

  headless         = true

  http_directory   = "http"

  boot_wait        = "5s"
  boot_command     = [
    "<down><tab><wait>",
    "auto=true ",
    "priority=critical ",
    "url=http://{{ .HTTPIP }}:{{ .HTTPPort }}/preseed.cfg ",
    "hostname=osvbng ",
    "domain=local ",
    "locale=en_US.UTF-8 ",
    "keymap=us ",
    "netcfg/get_hostname=osvbng ",
    "netcfg/get_domain=local ",
    "<enter>"
  ]

  ssh_username     = "root"
  ssh_password     = "osvbng"
  ssh_timeout      = "30m"

  shutdown_command = "echo 'osvbng' | sudo -S shutdown -P now"
}

build {
  sources = ["source.qemu.osvbng"]

  provisioner "shell" {
    inline = ["mkdir -p /tmp/vpp-plugins"]
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

  provisioner "shell" {
    environment_vars = [
      "VPP_VERSION=${var.vpp_version}",
      "FRR_VERSION=${var.frr_version}",
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
    output = "output/${var.vm_name}.qcow2.gz"
  }
}
