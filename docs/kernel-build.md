# Firecracker Guest Kernel Build Guide

This document outlines the procedure for compiling a production-grade, hypervisor-optimized Linux 6.1 guest kernel. The
output of this pipeline is a raw, uncompressed vmlinux ELF binary. This artifact is dynamically downloaded by the scaled
edge daemon during the provisioning phase to boot customer microVMs via Firecracker on active nodes.

## Build Dependencies

Run the following command as root on a Debian host.

```shell
apt update
apt install -y build-essential bc flex bison kmod cpio libncurses-dev libelf-dev libssl-dev wget tar
```                 

**Dependency breakdown:**
Understanding the toolchain is critical for debugging platform builds. Here is exactly what each package does during the
kernel compilation process:

- `build-essential`: A meta-package that installs the GNU C Compiler (gcc) and the make utility.
- `bc`: An arbitrary-precision command-line calculator relied on by the kernel Makefile.
- `flex` and `bison`: A lexical analyzer and parser generator. The kernel build system uses these to parse Kconfig
  files.
- `kmod`: Tools for managing Linux kernel modules like `depmod`. Even for a monolithic hypervisor build, the build system
  requires these utilities to map module dependencies.
- `libncurses-dev`: Provides the API for text-based terminal UIs. Required if we need to modify kernel flags.
- `libelf-dev`: Development libraries for parsing and generating ELF Executable and Linkable Format files.
- `libssl-dev`: OpenSSL headers. The modern Linux kernel uses OpenSSL during the build process to handle the cryptographic
  tasks.

## Source Code Retrieval

We strictly build from the kernel.org LTS branch for stability. For hypervisor compatibility we target Linux 6.1.x.

```shell
# navigate to standard src directory
cd /usr/src

# Download the latest 6.1 source tarball
wget https://cdn.kernel.org/pub/linux/kernel/v6.x/linux-6.1.80.tar.xz

# extract the archive
tar -xf linux-6.1.80.tar.xz

# enter build directory
cd linux-6.1.80
```

## Configuration

We do not use the default Linux config, as it contains thousands of unnecessary drivers for physical hardware. We use the
officially maintained Firecracker microVM configuration, which strips down the kernel directly to the virtio drivers
required for hypervisor execution.
```shell
# Download the AWS-maintained Firecracker configuration for 6.1
wget https://raw.githubusercontent.com/firecracker-microvm/firecracker/main/resources/guest_configs/microvm-kernel-ci-x86_64-6.1.config -O .config

# Validate the config and accept default values for any newly introduced kernel flags
make olddefconfig
```

## Compilation
Compile the kernel. We specify the vmlinux target to ensure we get an uncompressed binary.
```shell
# Execute the build utilizing all available CPU cores
make -j$(nproc) vmlinux
```

## Artifact Extraction & Deployment
```shell
file vmlinux
```

Once done we can move to our Object storage.
