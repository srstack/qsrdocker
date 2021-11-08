## Docker/Run 的简单实现

### qsrdocker 

		./qsrdocker -h
		NAME:
		   qsrdocker -  qsrdocker is a simple container runtime implementation.

		USAGE:
			qsrdocker is a simple container runtime implementation.

		VERSION:
		   1.2.1

		COMMANDS:
		   run      Create a container with namespace and cgroup
		   commit   commit a container into image
		   ps       List all the container
		   logs     Print logs of a container
		   exec     Exec a command into container
		   inspect  Print info of a container
		   stop     Stop a container
		   rm       Remove unused one or more containers
		   start    Start one or more stopped containers
		   image    qsrdocker image COMMAND
		   network  qsrdocker network COMMAND
		   help, h  Shows a list of commands or help for one command

		GLOBAL OPTIONS:
		   --help, -h     show help
		   --version, -v  print the version

### qsrdocker run 

		./qsrdocker run -h
		NAME:
		   qsrdocker run - Create a container with namespace and cgroup

		USAGE:
		   qsrdocker run [command options] imageName [command]

		OPTIONS:
		   --it, --ti                Enable tty and Keep STDIN open even if not attached
		   -d                        Detach container
		   -m value                  Set Memory limit
		   --cpushare value          Set cpushare limit
		   --cpuset value            Set cpuset limit
		   --cpumem value            Set cpumem node limit in NUMA mode，Usually no restrictions
		   --name value              Container name
		   --oom_kill_disable value  oom_kill_disable, 1: disable 0:able (default 0)
		   -v value                  Set volume mount
		   -e value                  Set environment
		   -n value                  Set container network id (default: "qsrdocker0")
		   --netdriver value         Set container network driver, like bridge, host, none, container (default: "bridge")
		   --container value         Set container ID/Name with container driver network (default: "qsrdocker0")
		   -p value                  Set port mapping
		   
		# test
		./qsrdocker run -d -cpuset 0 -m 100m -name heroyf -p 110:80  nginx:v1
		3lsx66n203


### qsrdocker commit
		./qsrdocker commit -h
		NAME:
		   qsrdocker commit - commit a container into image

		USAGE:
		   qsrdocker commit containerName imageName

### qsrdocker ps

		 ./qsrdocker ps -h
		NAME:
		   qsrdocker ps - List all the container

		USAGE:
		   qsrdocker ps [command options]

		OPTIONS:
		   -a  Show all containers (default shows just running)

		# test
		./qsrdocker ps -a 
		CONTAINER ID   IMAGE       NAME        PID         STATUS      COMMAND     UP TIME       CREATED
		3lsx66n203     nginx:v1    heroyf      2733        Running     nginx       Up 157 Days   2020-03-14 19:03:51


### qsrdocker exec

		./qsrdocker exec -h
		NAME:
		   qsrdocker exec - Exec a command into container

		USAGE:
		   qsrdocker exec [command options] containerName [command]

		OPTIONS:
		   --it, --ti  Enable tty and Keep STDIN open even if not attached
		
		# test
		./qsrdocker exec -it 3lsx66n203  /bin/sh
		/ # ifconfig 
		bridge-3lsx6 Link encap:Ethernet  HWaddr 2A:DF:30:32:54:C1  
				  inet addr:172.20.0.2  Bcast:172.20.0.255  Mask:255.255.255.0
				  inet6 addr: fe80::28df:30ff:fe32:54c1/64 Scope:Link
				  UP BROADCAST RUNNING MULTICAST  MTU:1500  Metric:1
				  RX packets:11697 errors:0 dropped:0 overruns:0 frame:0
				  TX packets:13811 errors:0 dropped:0 overruns:0 carrier:0
				  collisions:0 txqueuelen:1000 
				  RX bytes:761870 (744.0 KiB)  TX bytes:1068106 (1.0 MiB)

		lo        Link encap:Local Loopback  
				  inet addr:127.0.0.1  Mask:255.0.0.0
				  inet6 addr: ::1/128 Scope:Host
				  UP LOOPBACK RUNNING  MTU:65536  Metric:1
				  RX packets:0 errors:0 dropped:0 overruns:0 frame:0
				  TX packets:0 errors:0 dropped:0 overruns:0 carrier:0
				  collisions:0 txqueuelen:1000 
				  RX bytes:0 (0.0 B)  TX bytes:0 (0.0 B)

		/ # exit


### qsrdocker logs


		./qsrdocker logs -h
		NAME:
		   qsrdocker logs - Print logs of a container

		USAGE:
		   qsrdocker logs [command options] containerName

		OPTIONS:
		   -f, --follow            Follow log output
		   -t value, --tail value  Show from the end of the logs (default "all") (default: 0)


### qsrdocker inspect
		./qsrdocker inspect 3lsx66n203
		{
			 "ID": "3lsx66n203",
			 "Name": "heroyf",
			 "CreateTime": "2020-03-14 19:03:51",
			 "Status": {
				 "Pid": 2733,
				 "Status": "Running",
				 "Running": true,
				 "Paused": false,
				 "OOMKilled": false,
				 "Dead": false,
				 "StartTime": "2020-03-14 19:03:51"
			 },
			 "Driver": "overlay2",
			 "GraphDriver": {
				 "Driver": "overlay2",
				 "Data": {
					 "LowerDir": "/var/qsrdocker/image/CG3Y24MV89:/var/qsrdocker/image/0GACZ82691:/var/qsrdocker/image/QY18CR632Q:/var/qsrdocker/image/WVLZ001ON7:/var/qsrdocker/image/FS3259SD89S32DSF74",
					 "MergedDir": "/var/qsrdocker/overlay2/3lsx66n203/merged",
					 "UpperDir": "/var/qsrdocker/overlay2/3lsx66n203/diff",
					 "WorkDir": "/var/qsrdocker/overlay2/3lsx66n203/work"
				 }
			 },
			 "Mount": [
				 {
					 "Type": "bind",
					 "Source": "/var/qsrdocker/container/3lsx66n203/hosts",
					 "Destination": "/etc/hosts",
					 "RW": true
				 },
				 {
					 "Type": "bind",
					 "Source": "/var/qsrdocker/container/3lsx66n203/hostname",
					 "Destination": "/etc/hostname",
					 "RW": true
				 },
				 {
					 "Type": "bind",
					 "Source": "/var/qsrdocker/container/3lsx66n203/resolv.conf",
					 "Destination": "/etc/resolv.conf",
					 "RW": true
				 }
			 ],
			 "Cgroup": {
				 "Path": "3lsx66n203",
				 "Resource": {
					 "MemoryLimit": "100m",
					 "CPUShare": "",
					 "CPUSet": "0",
					 "CPUMem": "",
					 "OOMKillDisable": "1"
				 }
			 },
			 "Tty": false,
			 "Image": "nginx:v1",
			 "Path": "nginx",
			 "Args": [],
			 "Env": [
				 "author=qsr",
				 "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
				 " master process nginx"
			 ],
			 "NetWorkConfig": {
				 "EndPointID": "3lsx66n203-qsrdocker0",
				 "Dev": {
					 "Index": 8,
					 "MTU": 0,
					 "TxQLen": -1,
					 "Name": "qsrveth3lsx6",
					 "HardwareAddr": null,
					 "Flags": 0,
					 "RawFlags": 0,
					 "ParentIndex": 0,
					 "MasterIndex": 4,
					 "Namespace": null,
					 "Alias": "",
					 "Statistics": null,
					 "Promisc": 0,
					 "Xdp": null,
					 "EncapType": "",
					 "Protinfo": null,
					 "OperState": 0,
					 "NetNsID": 0,
					 "NumTxQueues": 0,
					 "NumRxQueues": 0,
					 "GSOMaxSize": 0,
					 "GSOMaxSegs": 0,
					 "Vfs": null,
					 "Group": 0,
					 "Slave": null,
					 "PeerName": "bridge-3lsx6",
					 "PeerHardwareAddr": null
				 },
				 "VethName": "qsrveth3lsx6",
				 "IPAddress": "172.20.0.2",
				 "MACAddress": "",
				 "NetWork": {
					 "NETWORK ID": "qsrdocker0",
					 "IP Range": "172.20.0.1/24",
					 "GateWay IP": "172.20.0.1",
					 "NetDriver": "Bridge"
				 },
				 "Ports": {
					 "80/tcp": [
						 {
							 "HostIP": "0.0.0.0",
							 "HostPort": "110"
						 }
					 ]
				 }
			 }
		 }

### qsrdocker stop
		./qsrdocker stop -h
		NAME:
		   qsrdocker stop - Stop a container

		USAGE:
		   qsrdocker stop [command options] containerName

		OPTIONS:
		   -t value  Seconds to wait for stop before killing it (default: 0)
		   
		# test
		./qsrdocker stop 3lsx66n203
		./qsrdocker ps
		CONTAINER ID   IMAGE       NAME        PID         STATUS      COMMAND     UP TIME     CREATED
		./qsrdocker ps -a
		CONTAINER ID   IMAGE       NAME        PID         STATUS      COMMAND     UP TIME     CREATED
		3lsx66n203     nginx:v1    heroyf      -1          Paused      nginx       NULL        2020-03-14 19:03:51


### qsrdocker start

		./qsrdocker start -h
		NAME:
		   qsrdocker start - Start one or more stopped containers

		USAGE:
		   qsrdocker start containerName...
		   
		# test
		./qsrdocker start 3lsx66n203
		3lsx66n203
		./qsrdocker ps
		CONTAINER ID   IMAGE       NAME        PID         STATUS      COMMAND     UP TIME        CREATED
		3lsx66n203     nginx:v1    heroyf      7228        Running     nginx       Up 4 Seconds   2020-03-14 19:03:51

### qsrdocker image

		./qsrdocker image -h
		NAME:
		   qsrdocker image - qsrdocker image COMMAND

		USAGE:
		   qsrdocker image command [command options] [arguments...]

		COMMANDS:
		   ls  List images

		OPTIONS:
		   --help, -h  show help
		
		# test
		./qsrdocker image ls
		IMAGE NAME          TAG                 IMAGE ID             SIZE                CREATE TIME
		alpine              last                FS3259SD89S32DSF74   5.33 MB             2019-12-26 23:09:25
		busybox             last                3824655              430.08 MB           2019-12-25 21:31:24
		nginx               last                CG3Y24MV89           99.34 MB            2020-03-14 19:03:51
		nginx               v1                  CG3Y24MV89           99.34 MB            2020-03-14 19:03:51
		qsrimage            v11                 WVLZ001ON7           5.33 MB             2020-01-05 21:38:55
		qsrimage            v12                 QY18CR632Q           5.33 MB             2020-01-05 22:59:06


### qsrdocker network

		./qsrdocker network -h
		NAME:
		   qsrdocker network - qsrdocker network COMMAND

		USAGE:
		   qsrdocker network command [command options] [arguments...]

		COMMANDS:
		   ls      List networks
		   create  create a container network
		   remove  Remove Network

		OPTIONS:
		   --help, -h  show help
		   
		 # test
		 ./qsrdocker network ls
		NETWORK ID          GateWay IP          IP Range            Driver
		qsrdocker0          172.20.0.1          172.20.0.1/24       Bridge
		
		# test 
		ifconfig  | grep qsr
		qsrdocker0: flags=4163<UP,BROADCAST,RUNNING,MULTICAST>  mtu 1500
		qsrveth3lsx6: flags=4163<UP,BROADCAST,RUNNING,MULTICAST>  mtu 1500
		
		# test
		iptables -S -t nat | grep qsr 
		-A POSTROUTING -s 172.20.0.0/24 ! -o qsrdocker0 -j MASQUERADE
		-A QSRDOCKER -i qsrdocker0 -j RETURN
		-A QSRDOCKER ! -i qsrdocker0 -p tcp -m tcp --dport 110 -j DNAT --to-destination 172.20.0.2:80

### qsrdocker rm 
		./qsrdocker rm -h
		NAME:
		   qsrdocker rm - Remove unused one or more containers

		USAGE:
		   qsrdocker rm [command options] containerName...

		OPTIONS:
		   -f  Force the removal of a running container (uses SIGKILL)
		   -v  Remove the volumes associated with the container
