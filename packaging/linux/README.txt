Memory Harness 2.2.0 - Linux headless server
================================================

This package runs without a desktop, X11, Wayland or a browser.

1. Install and start:
   sudo ./install.sh

2. Verify:
   ./healthcheck.sh
   sudo systemctl status memory-harness --no-pager

3. Read logs:
   sudo journalctl -u memory-harness -n 100 --no-pager

4. Use MCP on the same server:
   Set MEMORYOS_AGENT_TOKEN through your secret manager, then configure the
   MCP command as:
     /usr/local/bin/memoryosd mcp --endpoint http://127.0.0.1:19777

5. Remote access:
   The service deliberately listens only on 127.0.0.1. Use an SSH tunnel:
     ssh -N -L 19777:127.0.0.1:19777 user@your-server

6. Upgrade:
   Extract the new package and run sudo ./install.sh again. Existing data and
   configuration are preserved.

7. Uninstall:
   sudo ./uninstall.sh
   Data remains under /var/lib/memory-harness for recovery.

Full guide:
https://github.com/luoyif/memory-harness/blob/main/docs/LINUX_SERVER.zh-CN.md
