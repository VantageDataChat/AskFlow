# Windows Service Installation Guide

Helpdesk now supports running as a Windows service with automatic startup.

## Installation Methods

### Method 1: Using the Installer (Recommended)

1. Run `build\build_installer.cmd` to create the installer
2. Run the generated `build\installer\helpdesk-installer.exe`
3. Follow the installation wizard
4. The service will be installed and started automatically

### Method 2: Manual Installation

#### Install the Service

```cmd
helpdesk.exe install --datadir="C:\ProgramData\Helpdesk\data"
```

Options:
- `--datadir=<path>`: Specify custom data directory (default: `./data`)

#### Start the Service

```cmd
helpdesk.exe start
```

Or use Windows Services Manager (`services.msc`) to start "Helpdesk Support Service"

#### Stop the Service

```cmd
helpdesk.exe stop
```

#### Uninstall the Service

```cmd
helpdesk.exe stop
helpdesk.exe remove
```

## Service Configuration

### Data Directory

The data directory stores:
- Configuration (`config.json`)
- Database files (SQLite)
- Uploaded documents
- Log files
- Images and videos

You can specify a custom data directory during installation or when manually installing the service.

### Logging

When running as a service, Helpdesk logs to:
1. **Windows Event Log**: Application log with source "HelpdeskService"
2. **File Log**: `<datadir>\logs\helpdesk.log`

View Windows event logs:
```cmd
eventvwr.msc
```

View file logs:
```cmd
type C:\ProgramData\Helpdesk\data\logs\helpdesk.log
```

### Port Configuration

The service listens on port 8080 by default. To change:
1. Edit `<datadir>\config.json`
2. Restart the service:
   ```cmd
   helpdesk stop
   helpdesk start
   ```

## Console Mode

You can still run Helpdesk in console mode (not as a service):

```cmd
helpdesk.exe
```

Or with custom data directory:

```cmd
helpdesk.exe --datadir="C:\CustomPath\data"
```

Console mode is useful for:
- Development and testing
- Running on user login (not system startup)
- Viewing real-time logs in the console

## CLI Commands

All CLI commands work regardless of service status:

```cmd
# Import documents
helpdesk.exe import --product <id> C:\Docs

# List products
helpdesk.exe products

# Backup database
helpdesk.exe backup --output C:\Backups

# Restore from backup
helpdesk.exe restore C:\Backups\helpdesk_full_*.tar.gz
```

## Troubleshooting

### Service Won't Start

1. Check Windows Event Log for errors:
   ```cmd
   eventvwr.msc
   ```

2. Check file log:
   ```cmd
   type C:\ProgramData\Helpdesk\data\logs\helpdesk.log
   ```

3. Verify data directory exists and is writable

4. Try running in console mode to see errors:
   ```cmd
   helpdesk.exe --datadir="C:\ProgramData\Helpdesk\data"
   ```

### Access Denied Errors

The service runs as LocalSystem by default and has full access.

If using a custom data directory, ensure the service account has read/write permissions.

### Port Already in Use

If port 8080 is already in use:
1. Stop the conflicting service
2. Or change Helpdesk port in `config.json`

### Cannot Install Service

Ensure you're running as Administrator:
- Right-click `cmd.exe` → "Run as administrator"
- Then run the install command

## Automatic Startup

When installed as a service, Helpdesk starts automatically on system boot.

To disable automatic startup:
1. Open `services.msc`
2. Find "Helpdesk Support Service"
3. Right-click → Properties
4. Change "Startup type" to "Manual" or "Disabled"

## Uninstalling

### Using the Installer

1. Open "Add or Remove Programs"
2. Find "Helpdesk Support Service"
3. Click "Uninstall"
4. Choose whether to keep or delete data directory

### Manual Uninstall

```cmd
helpdesk.exe stop
helpdesk.exe remove
```

Then manually delete:
- Installation directory (e.g., `C:\Program Files\Helpdesk`)
- Data directory (e.g., `C:\ProgramData\Helpdesk\data`)

## Building the Installer

Requirements:
- Go 1.18 or later
- NSIS 3.0 or later (https://nsis.sourceforge.io/)

Build command:
```cmd
cd D:\workprj\VantageSelfservice
build\build_installer.cmd
```

The installer will be created at:
```
build\installer\helpdesk-installer.exe
```

## Architecture

The Windows service implementation consists of:

- `internal/service/app_service.go` - Application service layer
- `internal/svc/service.go` - Windows service implementation
- `internal/svc/logger.go` - Dual logging (event log + file log)
- `main.go` - Command dispatcher and service entry point

The service runs the HTTP server in the background and handles Windows service control commands (start, stop, shutdown).
