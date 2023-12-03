go build -v -o /tmp .
ssh root@mullareros.com systemctl stop yd
scp /tmp/yd root@mullareros.com:/usr/bin
ssh root@mullareros.com systemctl start yd