## 🎨 Delivery config

You can customize the client before distributing it to friends by hosting your own `delivery_config.json`:

1. Copy the example from `/delivery/delivery_config.json`
2. Modify the values:
   ```json
   {
     "default_subscription_url": "https://your-site.com/config.json",
     "sing_box_license_file": "https://github.com/SagerNet/sing-box/raw/main/LICENSE",
     "in_archive_exec_path": "sing-box-1.12.0-rc.4-windows-amd64/sing-box.exe",
     "sing_box_zip_url": "https://github.com/SagerNet/sing-box/releases/download/v1.12.0-rc.4/sing-box-1.12.0-rc.4-windows-amd64.zip",
     "sing_box_version": "1.12.0-rc.4"
   }
   ```
3. Host it somewhere publicly accessible
4. Update `DeliveryConfigURL` in `config/constants.go` to point to your hosted file
5. Rebuild the client

This allows you to:
- Set a default subscription URL for your users
- Control which sing-box version gets downloaded
- Customize the license file source
