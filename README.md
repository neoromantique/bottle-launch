# Bottle Launch

A bash script to launch Flatpak applications with their data stored in encrypted `.bottle` files (LUKS containers). When unmounted, the data is opaque to the host filesystem.

## Features

- LUKS2 encryption with password authentication
- Automatic cleanup on exit, interrupt, or crash
- Concurrent access prevention via lock files
- Interactive bottle creation when missing
- Reuses existing mounts if bottle already open

## Dependencies

- `cryptsetup` (for LUKS)
- `flatpak`
- `coreutils` (truncate, sha256sum)
- sudo access

## Installation

```bash
# Copy to your PATH
sudo cp bottle-launch /usr/local/bin/
```

## Usage

### Create a new bottle

```bash
bottle-launch create keepass.bottle 500M
```

You'll be prompted to set an encryption password.

### Run an application

```bash
bottle-launch run keepass.bottle org.keepassxc.KeePassXC
```

If the bottle doesn't exist, you'll be prompted to create it with a size selection menu.

### Run with extra Flatpak arguments

```bash
# Database path is relative to bottle's HOME (the mount point)
bottle-launch run keepass.bottle org.keepassxc.KeePassXC -- ~/passwords.kdbx
```

### List mounted bottles

```bash
bottle-launch list
```

## How It Works

1. **Create**: Makes a sparse file, formats it as LUKS2, creates ext4 filesystem inside
2. **Run**: Opens LUKS container, mounts it, runs Flatpak with `HOME` and `XDG_*` pointing inside the mount
3. **Cleanup**: On app exit (or Ctrl+C), unmounts and closes the LUKS container

All application data (config, cache, local storage) goes into the encrypted bottle instead of `~/.var/app/<app-id>`.

## Security Notes

- Password is entered each launch (no keyfiles stored)
- When unmounted, bottle contents are encrypted at rest
- Each bottle gets a unique mapper name based on its absolute path hash
- Lock files prevent concurrent access to the same bottle

## Disclaimer

This code was generated with assistance from an LLM (Claude). Review before use in production. No warranty is provided.
