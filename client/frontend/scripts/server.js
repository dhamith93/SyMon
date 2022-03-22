document.addEventListener('DOMContentLoaded', ()=> {
    const menuBtn = document.querySelector('.hamburger');
    const navMenu = document.querySelector('.nav-menu');
    const serverNameElems = document.querySelectorAll('.server-name');
    const navLinks = document.querySelectorAll('.nav-link');
    const urlParams = new URLSearchParams(window.location.search);
    const serverName = urlParams.get('name');
    const systemTable = document.querySelector('#system-table');
    const procCPUTable = document.querySelector('#proc-cpu-table');
    const procMemTable = document.querySelector('#proc-memory-table');
    const servicesTable = document.querySelector('#services-table');
    const cpuCircle = document.querySelector('#cpu-circle');
    const cpuLoadAvgElem = document.querySelector('#cpu-load-avg');
    const memoryCircle = document.querySelector('#memory-circle');
    const memoryLoadElem = document.querySelector('#memory-load');
    const circleStrokeDashOffset = 472;
    const procHeaders = ['PID', 'CPU %', 'Memory %', 'Command'];

    serverNameElems.forEach(elem => {
        elem.innerHTML = encodeURIComponent(urlParams.get('name'));
    });

    menuBtn.addEventListener('click', () => {
        menuBtn.classList.toggle('is-active');
        navMenu.classList.toggle('is-active');
    });

    navLinks.forEach(link => {
        link.addEventListener('click', (e) => {
            navLinks.forEach(link => {
                link.classList.remove('link-active');
            });
            link.classList.add('link-active');
        });
    });
    
    loadSystem = () => {
        axios.get('/system?serverId='+serverName).then((response) => {
            if (response.data.Status == 'OK') {
                let system = response.data.Data;
                delete system.LoggedInUsers;
                populateTable(systemTable, system);
            }
        }, (error) => {
            console.error(error);
        });
    };

    loadCPU = (time = null) => {
        let url = '/proc?serverId='+serverName;
        if (time !== null) {
            url = url + '&time=' + time;
        }
        axios.get(url).then((response) => {
            if (response.data.Status == 'OK') {
                cpu = response.data.Data[0];
                cpuCircle.style.strokeDashoffset = circleStrokeDashOffset - circleStrokeDashOffset * (cpu.LoadAvg / 100);
                cpuLoadAvgElem.innerHTML = `${cpu.LoadAvg}%`;
            }
            // populateTable(cpuTable, response.data.Data);
        }, (error) => {
            console.error(error);
        }); 
    }

    loadMemory = (time = null) => {
        let url = '/memory?serverId='+serverName;
        if (time !== null) {
            url = url + '&time=' + time;
        }
        axios.get(url).then((response) => {
            if (response.data.Status == 'OK') {
                memory = response.data.Data;
                memoryCircle.style.strokeDashoffset = circleStrokeDashOffset - circleStrokeDashOffset * (memory.PercentageUsed / 100);
                memoryUsage = (memory.PercentageUsed).toFixed(2);
                memoryLoadElem.innerHTML = `${memoryUsage}%`;
            }
            // populateTable(cpuTable, response.data.Data);
        }, (error) => {
            console.error(error);
        }); 
    }

    loadProcesses = (time = null) => {
        let url = '/processes?serverId='+serverName;
        if (time !== null) {
            url = url + '&time=' + time;
        }
        axios.get(url).then((response) => {
            if (response.data.Data.CPU && response.data.Data.Memory) {
                cpuArr = [];
                memArr = [];

                response.data.Data.CPU.forEach(row => {
                    cpuArr.push([row.Pid, row.CPUUsage, row.MemUsage, row.ExecPath]);
                });

                response.data.Data.Memory.forEach(row => {
                    memArr.push([row.Pid, row.CPUUsage, row.MemUsage, row.ExecPath]);
                });

                handleProcessList(cpuArr, procCPUTable);
                handleProcessList(memArr, procMemTable);
            }
        }, (error) => {
            console.error(error);
        }); 
    }

    loadServices = (time = null) => {
        let url = '/services?serverId='+serverName;
        if (time !== null) {
            url = url + '&time=' + time;
        }
        axios.get(url).then((response) => {
            try {
                if (response.data.Data) {
                    let data = {}
                    response.data.Data.forEach(row => {
                        data[row['Name']] = row['Running'] ? '<i class="Small material-icons">check_circle</i>' : '<i class="Small material-icons">cancel</i>';
                    });
                    populateTable(servicesTable, data)
                }
            } catch (e) { }
        }, (error) => {
            console.error(error);
        }); 
    }

    handleProcessList = (usage, table) => {
        if (usage) {
            clearElement(table).then(() => {
                table.createTHead();
                let tr = document.createElement('tr');
                procHeaders.forEach(element => {
                    th = document.createElement('th');
                    th.innerHTML = element;
                    tr.appendChild(th);
                });
                table.tHead.appendChild(tr);
                usage.forEach(row => {
                    let tableRow = table.insertRow(-1);
                    let data = [row[0], row[1], row[2], row[3]];
                    data.forEach(i => {
                        let cell = tableRow.insertCell(-1);
                        cell.innerHTML = i;
                    });
                });
            });
        }
    }

    loadSystem();
    loadCPU();
    loadMemory();
    loadProcesses();
    loadServices();

    setInterval(() => {
        loadCPU();
        loadMemory();
        loadProcesses();
        loadServices();
    }, 6000);

});