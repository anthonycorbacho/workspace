`brew install dnsmasq`

`mkdir -pv $(brew --prefix)/etc/`

`echo 'address=/glassdome/127.0.0.1' >> $(brew --prefix)/etc/dnsmasq.conf`

`sudo brew services start dnsmasq`

`sudo mkdir -v /etc/resolver`

`sudo bash -c 'echo "nameserver 127.0.0.1" > /etc/resolver/glassdome'`

test --> `ping -c 1 another-sub-domain.glassdome` should return 127.0.0.1
