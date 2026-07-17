# Fresh Devotionals: Server Setup & Nginx Documentation

This document serves as your complete manual for the production server environment, detailing the system architecture, Nginx configurations, database connections, and CI/CD pipelines.

---

## 1. System Architecture

The application is deployed on a single DigitalOcean Ubuntu Droplet:
- **Port 80/443 (HTTP/HTTPS)**: Handled by Nginx.
  - Serves the static compiled React Admin files from `/var/www/fresh-words-admin`.
  - Proxies API traffic (`/api/v1/...`) to the local Go server.
- **Port 8080**: The Go backend server listening on localhost.
- **Port 5432**: PostgreSQL database listener, restricted to localhost connections only.

```
                  ┌────────────────────────────────────────────────────────┐
                  │                      Ubuntu VPS                        │
                  │                                                        │
   HTTPS (:443)   │   ┌───────────────┐        ┌───────────────────────┐   │
 ────────────────>│   │     Nginx     │───────>│  React Admin Panel    │   │
                  │   └───────┬───────┘        │ (Static /var/www/...) │   │
                  │           │                └───────────────────────┘   │
                  │  Proxy    │                                            │
                  │  (:8080)  ▼                                            │
                  │   ┌───────────────┐                                    │
                  │   │   Go Server   │                                    │
                  │   └───────┬───────┘                                    │
                  │           │                                            │
                  │           │ Local Link (:5432)                         │
                  │           ▼                                            │
                  │   ┌───────────────┐                                    │
                  │   │  PostgreSQL   │                                    │
                  │   └───────────────┘                                    │
                  └────────────────────────────────────────────────────────┘
```

---

## 2. Nginx Configuration Demystified

The Nginx configuration file is located at `/etc/nginx/sites-available/fresh-words`. Here is the complete commented breakdown:

```nginx
server {
    server_name freshdevotionals.com www.freshdevotionals.com;

    # 1. ALLOW LARGE UPLOADS
    # By default, Nginx limits uploads to 1MB. We increase this to 250MB to support large devotional documents.
    client_max_body_size 250M;

    # 2. REACT ADMIN PANEL ROUTING
    # Tells Nginx where to find static React files.
    # 'try_files $uri $uri/ /index.html' is critical for SPAs. If a user requests a path directly (like /privacy),
    # Nginx serves index.html and lets React Router handle the view rendering in the browser.
    location / {
        root /var/www/fresh-words-admin;
        index index.html index.htm;
        try_files $uri $uri/ /index.html;
    }

    # 3. GO BACKEND API REVERSE PROXY
    # Intercepts any incoming requests starting with "/api/" and proxies them to the Go backend.
    location /api/ {
        proxy_pass http://127.0.0.1:8080/api/;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # 4. PREVENT TIMEOUTS FOR LARGE UPLOADS
        # Increases connection, send, and read limits to 10 minutes (600s) to allow slow/large file uploads to complete.
        proxy_connect_timeout 600s;
        proxy_send_timeout    600s;
        proxy_read_timeout    600s;
        send_timeout          600s;
    }

    # 5. SSL CONFIGURATION (Managed Automatically by Certbot)
    listen 443 ssl;
    ssl_certificate /etc/letsencrypt/live/freshdevotionals.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/freshdevotionals.com/privkey.pem;
    include /etc/letsencrypt/options-ssl-nginx.conf;
    ssl_dhparam /etc/letsencrypt/ssl-dhparams.pem;
}

# 6. HTTP TO HTTPS AUTOMATIC REDIRECT
# Automatically catches port 80 traffic (HTTP) and redirects to secure port 443 (HTTPS).
server {
    if ($host = www.freshdevotionals.com) {
        return 301 https://$host$request_uri;
    }
    if ($host = freshdevotionals.com) {
        return 301 https://$host$request_uri;
    }

    listen 80;
    server_name freshdevotionals.com www.freshdevotionals.com;
    return 404;
}
```

---

## 3. Go Server Service Configuration (systemd)

The Go backend runs as a background process using Linux's standard `systemd` daemon. 

### The service configuration file (`/etc/systemd/system/fresh-words.service`)
```ini
[Unit]
Description=Fresh Words Devotionals Go Backend
After=network.target postgresql.service

[Service]
Type=simple
User=www-data
WorkingDirectory=/var/www/fresh-words-backend
ExecStart=/var/www/fresh-words-backend/fresh-words-server
Restart=always
EnvironmentFile=/var/www/fresh-words-backend/.env

[Install]
WantedBy=multi-user.target
```

### Control Commands:
- **Start the service**: `sudo systemctl start fresh-words`
- **Stop the service**: `sudo systemctl stop fresh-words`
- **Restart the service**: `sudo systemctl restart fresh-words`
- **View active logs**: `sudo journalctl -u fresh-words --no-pager -n 50`

---

## 4. PostgreSQL Configuration

The database runs natively on the Droplet.
- **Port**: `5432` (restricted to localhost `127.0.0.1` inside `postgresql.conf` for security).
- **Access database CLI**: `sudo -i -u postgres psql`
- **Backup database command**:
  ```bash
  pg_dump -U fresh_words_user -d fresh_words_db -h 127.0.0.1 -F c -b -v -f /backups/fresh_words_db.backup
  ```

---

## 5. Zero-Downtime CI/CD Deployments

In GitHub Actions, trying to overwrite a running binary results in a `Text file busy` error. We solved this in the deploy workflow using a **Linux Unlink/Rename** step before copying:

1. **Renaming**: The workflow SSHs into the server and renames the running binary to `.old`:
   ```bash
   mv fresh-words-server fresh-words-server.old
   ```
   *Linux allows you to rename or delete a file even if it's currently running.*
2. **SCP Copy**: The workflow copies the newly built `fresh-words-server` binary over to `/var/www/fresh-words-backend` without conflicts.
3. **Restart**: The workflow restarts the `fresh-words` service (loading the new binary into memory) and deletes the `.old` file.
