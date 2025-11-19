# Task Naming Parity Analysis: Local vs GCP

## ✅ IMPLEMENTATION COMPLETE

**Status**: All high-priority naming inconsistencies have been resolved.  
**Date**: 2025-11-06  
**Result**: Local and GCP taskfiles now have 1:1 naming parity for common operations.

---

## Overview
This document analyzed naming inconsistencies between local and GCP taskfiles to achieve 1:1 parity where possible.

## Architecture Context
- **Local**: Single-machine deployment with all services in one docker-compose.yml
- **GCP**: Distributed deployment with 2 instances (backend + ws-server)

Some differences are **expected** due to architecture. Others were **inconsistencies** that have been fixed.

---

## Changes Applied

### ✅ Phase 1: High Priority Renames (GCP) - COMPLETE

1. **✅ Renamed update → rebuild**
   - `deployment:update:backend` → `deployment:rebuild:backend`
   - `deployment:update:ws` → `deployment:rebuild:ws`
   - `deployment:update:all` → `deployment:rebuild:all`
   - Added legacy aliases for backwards compatibility

2. **✅ Moved quick-start to deployment**
   - `services:quick-start` → `deployment:quick-start`
   - Updated `up` shortcut to call `deployment:quick-start`

3. **✅ Moved logs to services**
   - `health:logs:backend` → `services:logs:backend`
   - `health:logs:ws` → `services:logs:ws`
   - `health:logs:backend:tail` → `services:logs:backend:tail`
   - `health:logs:ws:tail` → `services:logs:ws:tail`

4. **✅ Added ps shortcut**
   - Added `services:ps` → `services:ps:all`

5. **✅ Added show-urls alias**
   - Added `deployment:show-urls` → `stats:urls`

### ✅ Phase 2: Add Missing Tasks (GCP) - COMPLETE

6. **✅ Added deployment:setup**
   ```yaml
   setup:
     desc: Complete GCP deployment from scratch
     cmds:
       - task: infrastructure
       - task: guided-setup
   ```

7. **✅ Added deployment:stop**
   ```yaml
   stop:
     desc: Stop all services (keeps volumes)
     cmds:
       - task: ../services:stop:all
       - echo "✅ Services stopped (containers removed, infrastructure preserved)"
   ```

8. **✅ Added deployment:reset**
   ```yaml
   reset:
     desc: Reset deployment (stop + rebuild + start - for config changes)
     cmds:
       - echo "⚠️  Resetting deployment (stop + rebuild + start)..."
       - echo "   Press Ctrl+C to cancel, or wait 5 seconds..."
       - sleep 5
       - task: ../services:stop:all
       - echo "🔨 Rebuilding services..."
       - task: rebuild:all
       - echo "⏳ Waiting for services to be ready..."
       - sleep 15
       - task: ../health:all
       - echo "✅ Deployment reset complete"
   ```

9. **✅ Added deployment:rebuild-code**
   ```yaml
   rebuild-code:
     desc: Rebuild services after code changes
     cmds:
       - echo "🔨 Rebuilding services..."
       - task: rebuild:all
       - echo "🔍 Checking health..."
       - sleep 5
       - task: ../health:all
       - echo "✅ Rebuild complete"
   ```

### ✅ Phase 3: Add Top-level Shortcuts (Both) - COMPLETE

10. **✅ Added to root Taskfile.yml (works for both local and GCP context)**
    ```yaml
    up:          # Quick start deployment
    down:        # Stop deployment
    restart:     # Restart all services
    status:      # Show deployment status
    verify:      # Verify health
    deploy:      # Complete deployment from scratch
    ```

---

## Final Task Structure

### Local Taskfiles
```
taskfiles/v1/local/
├── services.yml       - Service management (start, stop, restart, rebuild, delete, logs, ps)
├── deployment.yml     - Deployment workflows (setup, reset, quick-start, stop, rebuild-code)
└── health.yml         - Health checks (all services)
```

### GCP Taskfiles
```
taskfiles/v1/gcp/
├── Taskfile.yml       - Orchestrator + top-level shortcuts (up, down, restart, status, verify, deploy)
├── services.yml       - Service management (start, stop, restart, ps, logs)
├── deployment.yml     - Infrastructure + workflows (setup, infrastructure, guided-setup, quick-start, stop, reset, rebuild-code, rebuild:*, show-urls, create:*, firewall:*, setup:*, ssh:*)
├── health.yml         - Health checks (backend, ws, all)
├── stats.yml          - Statistics and URLs
└── load-test.yml      - Load testing
```

---

## Task Mapping Table (Final)

| Functionality | Local | GCP | Status |
|--------------|-------|-----|--------|
| Complete setup | `deployment:setup` | `deployment:setup` | ✅ Same |
| Quick start | `deployment:quick-start` | `deployment:quick-start` | ✅ Same |
| Stop all | `deployment:stop` | `deployment:stop` | ✅ Same |
| Restart all | `services:restart:all` | `services:restart:all` | ✅ Same |
| Rebuild code | `services:rebuild:*` | `deployment:rebuild:*` | ✅ Consistent naming |
| Reset deployment | `deployment:reset` | `deployment:reset` | ✅ Same |
| Show URLs | `deployment:show-urls` | `deployment:show-urls` | ✅ Same |
| Service logs | `services:logs:*` | `services:logs:*` | ✅ Same |
| Container status | `services:ps` | `services:ps` | ✅ Same |
| Health check | `health:all` | `health:all` | ✅ Same |
| Top-level up | `up` | `up` | ✅ Same |
| Top-level down | `down` | `down` | ✅ Same |
| Top-level restart | `restart` | `restart` | ✅ Same |
| Top-level status | `status` | `status` | ✅ Same |
| Top-level verify | `verify` | `verify` | ✅ Same |
| Top-level deploy | `deploy` | `deploy` | ✅ Same |

---

## Expected Differences (Acceptable)

### ✅ Service Grouping
**Local**: Individual services (redpanda, console, publisher, ws, prometheus, grafana, loki, promtail)  
**GCP**: Grouped services (backend, ws)  
**Reason**: Distributed architecture requires instance-level grouping  
**Action**: No change needed

### ✅ Infrastructure Tasks
**Local**: No infrastructure tasks (docker-compose handles it)  
**GCP**: `deployment:create:*`, `deployment:firewall:*`, `deployment:reserve-ip:*`  
**Reason**: GCP requires explicit infrastructure provisioning  
**Action**: No change needed

### ✅ Delete Tasks
**Local**: `services:delete:*` (for cleanup)  
**GCP**: No delete tasks  
**Reason**: GCP instances persist, only containers restart  
**Action**: No change needed (acceptable difference)

---

## Documentation Updates

### ✅ Updated Files
1. **taskfiles/v1/gcp/deployment.yml** - Renamed update→rebuild, added missing tasks
2. **taskfiles/v1/gcp/services.yml** - Moved quick-start out, added ps shortcut, added logs section
3. **taskfiles/v1/gcp/health.yml** - Removed logs (moved to services)
4. **taskfiles/v1/gcp/Taskfile.yml** - Updated shortcuts to reference new task locations
5. **Taskfile.yml (root)** - Added top-level shortcuts, updated help menu
6. **GCP_CONSOLIDATION_PLAN.md** - Updated all task references, added new workflows
7. **TASK_NAMING_ANALYSIS.md** - This document (analysis + completion status)

---

## Success Criteria - ALL MET ✅

✅ Users can use same task names for common operations on both local and GCP  
✅ Task names accurately describe what they do (rebuild vs update)  
✅ Tasks are organized in the same files (logs in services, not health)  
✅ Both local and GCP have convenient top-level shortcuts  
✅ Documentation reflects consistent naming  
✅ Legacy aliases provide backwards compatibility  
✅ Help menu updated with new task organization

---

## Usage Examples

### Common Operations (Work identically for local and GCP)

```bash
# First-time setup
task local:deploy:setup    # or task deploy (when in local context)
task gcp:deploy:setup      # or task gcp:deploy

# Daily operations
task local:up              # or task up
task gcp:up                # or task gcp:up

task local:down            # or task down
task gcp:down              # or task gcp:down

task local:restart         # or task restart
task gcp:restart           # or task gcp:restart

task local:status          # or task status
task gcp:status            # or task gcp:status

# Code changes
task local:services:rebuild:ws
task gcp:deployment:rebuild:ws

# Logs
task local:services:logs:ws
task gcp:services:logs:ws

# Container status
task local:services:ps
task gcp:services:ps
```

---

## Migration Path for Existing Users

### If you were using old task names:
```bash
# Old → New (still works via aliases)
task gcp:deployment:update:backend  → task gcp:deployment:rebuild:backend
task gcp:deployment:update:ws       → task gcp:deployment:rebuild:ws
task gcp:services:quick-start       → task gcp:deployment:quick-start
task gcp:health:logs:backend        → task gcp:services:logs:backend
task gcp:services:ps:all            → task gcp:services:ps
```

**Note**: Old task names still work but show deprecation warnings. Update to new names for best experience.

---

## Benefits Achieved

1. **Consistency**: Same operation = same task name across environments
2. **Predictability**: Users familiar with local tasks can use GCP with no learning curve
3. **Clarity**: Task names accurately describe what they do (rebuild vs update)
4. **Organization**: Related tasks grouped together (logs with services, not health)
5. **Convenience**: Top-level shortcuts for common operations
6. **Discoverability**: Better help menu structure
7. **Maintainability**: Clear organization makes future changes easier
8. **Backwards Compatibility**: Legacy aliases prevent breaking existing workflows

---

**Implementation Status**: ✅ COMPLETE  
**Next Steps**: Use the new task structure! Try `task --list` to see all available tasks.
