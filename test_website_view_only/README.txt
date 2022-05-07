To run the website (locally):

1. In your terminal, do "go run main.go".
2. In your local browser, enter "localhost:8080/view".
3. You should see your IP address and the edit button.
   If you have previously used the edit form, there should be persisted submitted stuff from before.
4. To edit, simply click the edit button, put something in the text box and submit.

Have fun!


References:
- https://golangcode.com/get-the-request-ip-addr/
- https://go.dev/doc/articles/wiki/

=========================================================================================================

To run the website on Microsoft Azure:
   - ensure that port (80) is open

1. Enter the server terminal and clone the repository:
   > git clone https://github.students.cs.ubc.ca/CPSC416-2021W-T2/cpsc416_proj_mo3697_carsonh1_gkhui_ohryan55_bdai00_linushc.git

2. Install Go by using the following commands (refer to this if it says go not installed):
   > cd ~
   > wget https://go.dev/dl/go1.16.7.linux-amd64.tar.gz
   > tar -xvzf go1.16.7.linux-amd64.tar.gz
   > export PATH=$PATH:~/go/bin

3. Install nginx (https://www.digitalocean.com/community/tutorials/how-to-install-nginx-on-ubuntu-18-04):
   > sudo apt update
   > sudo apt install nginx
   > sudo ufw app list (check that there is Nginx Full, Nginx HTTP and Nginx HTTPS)

   > sudo ufw allow 'Nginx HTTP'
   > sudo ufw allow 'OpenSSH'
   > sudo ufw enable
   > sudo ufw status (check that there is Nginx HTTP, OpenSSH and their v6 counterparts)

   > systemctl status nginx (check for "active (running)")

4. Set up a reverse proxy with Nginx (https://www.digitalocean.com/community/tutorials/how-to-deploy-a-go-web-application-using-nginx-on-ubuntu-18-04):
   > cd /etc/nginx/sites-available
   > sudo nano [Azure machine IP address]
     Paste the following into the editor:

server {
   server_name [Azure server's IP];

   location / {
      proxy_pass http://localhost:[port, though 8080 works];
      proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
   }
}

   > sudo ln -s /etc/nginx/sites-available/[Azure server's IP] /etc/nginx/sites-enabled/[Azure server's IP]
   > sudo nginx -s reload

5. Turn on the website:
   > cd ~/cpsc416_proj_mo3697_carsonh1_gkhui_ohryan55_bdai00_linushc/test_website
   > go run main.go

Troubleshooting:
   - Weird nil pointer dereference:
      - Check to see if the files have written correctly.