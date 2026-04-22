# Copyright 2025 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

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
  default = "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img"
}

variable "base_image_checksum" {
  type    = string
  default = "file:https://cloud-images.ubuntu.com/noble/current/SHA256SUMS"
}

variable "vm_name" {
  type    = string
  default = "test-runner"
}

variable "containerlab_version" {
  type    = string
  default = "0.73.0"
}

variable "accelerator" {
  type    = string
  default = "kvm"
}

source "qemu" "test-runner" {
  iso_url      = var.base_image_url
  iso_checksum = var.base_image_checksum
  disk_image   = true

  output_directory = "output"
  vm_name          = "${var.vm_name}.qcow2"

  disk_size   = "30G"
  format      = "qcow2"
  accelerator = var.accelerator

  memory = 4096
  cpus   = 4

  headless = true

  cd_files = ["cloud-init/user-data", "cloud-init/meta-data"]
  cd_label = "cidata"

  ssh_username = "root"
  ssh_password = "testrunner"
  ssh_timeout  = "30m"

  shutdown_command = "shutdown -P now"
}

build {
  sources = ["source.qemu.test-runner"]

  provisioner "shell" {
    environment_vars = [
      "DEBIAN_FRONTEND=noninteractive",
      "CONTAINERLAB_VERSION=${var.containerlab_version}"
    ]
    scripts = [
      "scripts/configure-kernel.sh",
      "scripts/install-docker.sh",
      "scripts/install-containerlab.sh",
      "scripts/install-robot.sh",
      "scripts/cleanup.sh"
    ]
  }

  post-processor "compress" {
    output = "${var.vm_name}.qcow2.gz"
  }
}
