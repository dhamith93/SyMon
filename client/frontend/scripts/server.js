document.addEventListener('DOMContentLoaded', ()=> {
    const menuBtn = document.querySelector('.hamburger');
    const navMenu = document.querySelector('.nav-menu');
    const serverNameElems = document.querySelectorAll('.server-name');
    const navLinks = document.querySelectorAll('.nav-link');
    const sections = document.querySelectorAll('.section');
    const urlParams = new URLSearchParams(window.location.search);
    const serverName = urlParams.get('name');
    const systemTable = document.querySelector('#system-table');
    const procCPUTable = document.querySelector('#proc-cpu-table');
    const procMemTable = document.querySelector('#proc-memory-table');
    const procCPUTable2 = document.querySelector('#proc-cpu-table-2');
    const procMemTable2 = document.querySelector('#proc-memory-table-2');
    const servicesTable = document.querySelector('#services-table');
    const diskTable = document.querySelector('#disk-table');
    const networkTable = document.querySelector('#network-table');
    const networkInterfaceDropdown = document.querySelector('#network-interface-dropdown');
    const cpuCircle = document.querySelector('#cpu-circle');
    const cpuLoadAvgElem = document.querySelector('#cpu-load-avg');
    const memoryCircle = document.querySelector('#memory-circle');
    const memoryLoadElem = document.querySelector('#memory-load');
    const cpuTable = document.querySelector('#cpu-table');
    const memoryTable = document.querySelector('#memory-table');
    const swapTable = document.querySelector('#swap-table');
    const circleStrokeDashOffset = 472;
    const procHeaders = ['PID', 'CPU %', 'Memory %', 'Command'];
    const monitorInterval = 15;
    let selectedSection = 'overview-section';
    let firstTime = true;
    let isCPUFirstTime = true;
    let isMemFirstTime = true;
    let isDisksFirstTime = true;
    let isNetworkFirstTime = true;
    let cpuChart = null;
    let memChart = null;
    let diskPercentageChart = null;
    let networkChart = null;
    let selectedInterfaceIndex = 0;
    let orgRx = 0;
    let orgTx = 0;
    let toTime = 0;
    let fromTime = 0;

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
            selectedSection = link.dataset.section;
            
            sections.forEach(section => { 
                if (section.id === link.dataset.section) {
                    section.classList.add('section-active');    
                } else {
                    section.classList.remove('section-active');
                }
            });
            if (link.dataset.section !== 'cpu-section') {
                isCPUFirstTime = true;
                if (cpuChart !== null) {
                    cpuChart.destroy();
                    cpuChart = null;
                }
            }
            if (link.dataset.section !== 'memory-section') {
                isMemFirstTime = true;
                if (memChart !== null) {
                    memChart.destroy();
                    memChart = null;
                }
            }
            if (link.dataset.section !== 'disks-section') {
                isDisksFirstTime = true;
                if (diskPercentageChart !== null) {
                    diskPercentageChart.destroy();
                    diskPercentageChart = null;
                }
            }
            if (link.dataset.section !== 'networking-section') {
                isNetworkFirstTime = true;
                if (networkChart !== null) {
                    networkChart.destroy();
                    networkChart = null;
                }
            }
            loadData();
            menuBtn.classList.toggle('is-active');
            navMenu.classList.toggle('is-active');
        });
    });

    document.getElementById('cpu-chart-reset').addEventListener('click', () => {
        if (cpuChart !== null) {
            cpuChart.resetZoom();
        }
    });
    
    document.getElementById('mem-chart-reset').addEventListener('click', () => {
        if (memChart !== null) {
            memChart.resetZoom();
        }
    });

    document.getElementById('disks-percentage-chart-reset').addEventListener('click', () => {
        if (diskPercentageChart !== null) {
            diskPercentageChart.resetZoom();
        }
    });

    document.getElementById('networks-chart-reset').addEventListener('click', () => {
        if (networkChart !== null) {
            networkChart.resetZoom();
        }
    });

    networkInterfaceDropdown.addEventListener('change', e => {
        selectedInterfaceIndex = parseInt(e.target.value, 10);
        isNetworkFirstTime = true;
        if (networkChart !== null) {
            networkChart.destroy();
            networkChart = null;
        }
        loadNetworksBandwidth();
    });
    
    loadSystem = () => {
        axios.get('/system?serverId='+serverName).then((response) => {
            if (response.data.Status === 'OK') {
                let system = response.data.Data;
                toTime = system.Time;
                fromTime = toTime - 3600;
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
            if (response.data.Status === 'OK') {
                cpu = response.data.Data[0];
                if (selectedSection === 'overview-section') {
                    cpuCircle.style.strokeDashoffset = circleStrokeDashOffset - circleStrokeDashOffset * (cpu.LoadAvg / 100);
                    cpuLoadAvgElem.innerHTML = `${cpu.LoadAvg}%`;
                }
            }
            if (firstTime) {
                delete cpu.LoadAvg;
                delete cpu.CoreAvg;
                delete cpu.Time;
                populateTable(cpuTable, cpu);
            }
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
            if (response.data.Status === 'OK') {
                let memory = response.data.Data;
                memory.PercentageUsed = (memory.PercentageUsed).toFixed(2);

                if (selectedSection === 'overview-section') {
                    memoryCircle.style.strokeDashoffset = circleStrokeDashOffset - circleStrokeDashOffset * (memory.PercentageUsed / 100);
                    memoryLoadElem.innerHTML = `${memory.PercentageUsed}%`;
                }

                memory.Usage = `${memory.PercentageUsed}%`;
                memory.Available = `${memory.Available} ${memory.Unit}`;
                memory.Free = `${memory.Free} ${memory.Unit}`;
                memory.Total = `${memory.Total} ${memory.Unit}`;
                memory.Used = `${memory.Used} ${memory.Unit}`;
                delete memory.Unit;
                delete memory.PercentageUsed;
                populateTable(memoryTable, memory);
            }
        }, (error) => {
            console.error(error);
        }); 
    }

    loadSwap = (time = null) => {
        let url = '/swap?serverId='+serverName;
        if (time !== null) {
            url = url + '&time=' + time;
        }
        axios.get(url).then((response) => {
            let swap = response.data.Data;
            swap.PercentageUsed = (swap.PercentageUsed).toFixed(2);
            swap.Usage = `${swap.PercentageUsed}%`;
            swap.Free = `${swap.Free} ${swap.Unit}`;
            swap.Total = `${swap.Total} ${swap.Unit}`;
            swap.Used = `${swap.Used} ${swap.Unit}`;
            delete swap.Unit;
            delete swap.PercentageUsed;
            populateTable(swapTable, swap);
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
                let cpuArr = [];
                let memArr = [];

                response.data.Data.CPU.forEach(row => {
                    cpuArr.push([row.Pid, row.CPUUsage, row.MemUsage, row.ExecPath]);
                });

                response.data.Data.Memory.forEach(row => {
                    memArr.push([row.Pid, row.CPUUsage, row.MemUsage, row.ExecPath]);
                });

                handleProcessList(cpuArr, procCPUTable, procCPUTable2);
                handleProcessList(memArr, procMemTable, procMemTable2);
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

    loadCPUUsage = () => {
        let url = '/processor-usage-historical?serverId='+serverName+'&from='+fromTime+'&to='+toTime;
        if (!isCPUFirstTime) {
            url = '/processor-usage-historical?serverId='+serverName;
        }
        axios.get(url).then((response) => {
            let processedData = processUsageData(response.data.Data);
            if (!isCPUFirstTime && cpuChart !== null) {
                updateChart(cpuChart, processedData['labels'], processedData['data']);
                return;
            }
            isCPUFirstTime = false;
            if (cpuChart !== null) cpuChart.destroy();
            cpuChart = generateUsageChart(processedData, document.getElementById('cpu-usage-chart'), 'CPU', context => context.parsed.y + '%');
        }, (error) => {
            console.error(error);
        }); 
    }

    loadMemoryUsage = () => {
        let url = '/memory-historical?serverId='+serverName+'&from='+fromTime+'&to='+toTime;
        if (!isMemFirstTime) {
            url = '/memory-historical?serverId='+serverName;
        }
        axios.get(url).then((response) => {            
            let processedData = processUsageData(response.data.Data, 'memory');    
            if (!isMemFirstTime && memChart !== null) {
                updateChart(memChart, processedData['labels'], processedData['data']);
                return;
            }
            isMemFirstTime = false;
            if (memChart !== null) memChart.destroy();
            memChart = generateUsageChart(processedData, document.getElementById('memory-usage-chart'), 'MEM', context => context.parsed.y + '%');
        }, (error) => {
            console.error(error);
        }); 
    }

    loadDisks = (time = null) => {
        let url = '/disks?serverId='+serverName;
        if (time !== null) {
            url = url + '&time=' + time;
        }
        axios.get(url).then((response) => {
            if (response.data.Status === 'OK') {
                let disks = response.data.Data;
                handleDiskInfoTable(disks, diskTable);
                if (isDisksFirstTime) {
                    loadDiskUsage();
                }
                if (!isDisksFirstTime && diskPercentageChart !== null) {
                    updateChartForDisks(diskPercentageChart, response.data.Data);
                    return;
                }
            }
        }, (error) => {
            console.error(error);
        }); 
    }

    loadDiskUsage = () => {
        let url = '/disks-historical?serverId='+serverName+'&from='+fromTime+'&to='+toTime;
        axios.get(url).then((response) => {
            isDisksFirstTime = false;
            let processedData = processDisksForCharts(response.data.Data);
            if (diskPercentageChart !== null) diskPercentageChart.destroy();
            diskPercentageChart = generateUsageChartForMultiple(processedData, document.getElementById('disks-percentage-chart'), context => context.parsed.y + '%');
        }, (error) => {
            console.error(error);
        }); 
    }

    loadNetworks = (time = null) => {
        let url = '/network?serverId='+serverName;
        if (time !== null) {
            url = url + '&time=' + time;
        }
        axios.get(url).then((response) => {
            let networks = response.data.Data;
            if (networks[0]) {
                handleNetworkInfoTable(networks[0], networkTable);
                if (isNetworkFirstTime) {
                    loadNetworksBandwidth();
                    handleNetworkInterfaceDropdown(networks[0], networkInterfaceDropdown);
                }
                if (!isNetworkFirstTime && networkChart !== null) {
                    result = updateChartForNetwork(networkChart, networks[0], selectedInterfaceIndex, orgRx, orgTx, monitorInterval);
                    orgRx = result[0];
                    orgTx = result[1];
                }                
            }
        }, (error) => {
            console.error(error);
        }); 
    }

    loadNetworksBandwidth = () => {
        if (isNetworkFirstTime) {
            let url = '/network?serverId='+serverName+'&from='+fromTime+'&to='+toTime;
            axios.get(url).then((response) => {
                let data = response.data.Data;
                let result = processNetworksForCharts(data, selectedInterfaceIndex, monitorInterval);
                let processedData = result['data'];
                orgRx = result['orgRx'];
                orgTx = result['orgTx'];
                if (networkChart !== null) {
                    networkChart.destroy();
                }
                networkChart = generateUsageChartForMultiple(processedData, document.getElementById('networks-chart'), (context) => { context.parsed.y + 'kB/s'; }, false);
                isNetworkFirstTime = false;    
            }, (error) => {
                console.error(error);
            });            
        }
    }

    handleProcessList = (usage, table, tableInSection) => {
        if (usage) {
            if (selectedSection === 'overview-section') {
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

            if (selectedSection === 'cpu-section' || selectedSection === 'memory-section') {
                clearElement(tableInSection).then(() => {
                    tableInSection.createTHead();
                    let tr = document.createElement('tr');
                    procHeaders.forEach(element => {
                        th = document.createElement('th');
                        th.innerHTML = element;
                        tr.appendChild(th);
                    });
                    tableInSection.tHead.appendChild(tr);
                    usage.forEach(row => {
                        let tableRow = tableInSection.insertRow(-1);
                        let data = [row[0], row[1], row[2], row[3]];
                        data.forEach(i => {
                            let cell = tableRow.insertCell(-1);
                            cell.innerHTML = i;
                        });
                    });
                });
            }
        }
    }

    handleDiskInfoTable = (disks, table) => {
        let headerArray = ['Disk', 'Mounted', 'Type', 'Total', 'Used', 'Usage - space', 'Usage - inodes'];
        let diskArray = [];

        disks.forEach(disk => {
            diskArray.push([
                disk.FileSystem,
                disk.MountedOn,
                disk.Type,
                `${convertTo(disk.Usage.Size, 'B', 'M')} MB`,
                `${convertTo(disk.Usage.Used, 'B', 'M')} MB`,
                disk.Usage.Usage,
                disk.Inodes.Usage
            ])
        });

        clearElement(table).then(() => {
            table.createTHead();
            let tr = document.createElement('tr');
            headerArray.forEach(element => {
                th = document.createElement('th');
                th.innerHTML = element;
                tr.appendChild(th);
            });
            table.tHead.appendChild(tr);
            diskArray.forEach(row => {
                let tableRow = table.insertRow(-1);
                row.forEach(i => {
                    let cell = tableRow.insertCell(-1);
                    cell.innerHTML = i;
                });
            });
        });
    }

    handleNetworkInfoTable = (networks, table) => {
        let headerArray = ['Interface', 'IP', 'Rx', 'Tx'];
        let networkArray = [];
        networks.forEach(network => {
            networkArray.push([
                network.Interface,
                network.Ip,
                `${convertTo(network.Usage.RxBytes, 'B', 'M')} MB`,
                `${convertTo(network.Usage.TxBytes, 'B', 'M')} MB`
            ])
        });

        clearElement(table).then(() => {
            table.createTHead();
            let tr = document.createElement('tr');
            headerArray.forEach(element => {
                th = document.createElement('th');
                th.innerHTML = element;
                tr.appendChild(th);
            });
            table.tHead.appendChild(tr);
            networkArray.forEach(row => {
                let tableRow = table.insertRow(-1);
                row.forEach(i => {
                    let cell = tableRow.insertCell(-1);
                    cell.innerHTML = i;
                });
            });
        });
    }

    handleNetworkInterfaceDropdown = (networks, elem) => {
        let count = 0;
        networks.forEach(network => {
            elem.options[elem.options.length] = new Option(network.Interface, count);
            count += 1;
        });
    }

    loadData = () => {
        loadSystem();
        if (selectedSection === 'overview-section') {
            loadMemory();
            loadCPU();
            loadProcesses();
            loadServices();
        }

        if (selectedSection === 'cpu-section') {
            loadProcesses();
            loadCPUUsage();
        }

        if (selectedSection === 'memory-section') {
            loadProcesses();
            loadMemoryUsage();
            loadMemory();
            loadSwap();
        }

        if (selectedSection === 'disks-section') {
            loadDisks();            
        }

        if (selectedSection === 'networking-section') {
            loadNetworks();
        }
    }

    loadSystem();
    loadCPU();
    loadMemory();
    loadProcesses();
    loadServices();    

    setInterval(() => {
        firstTime = false;
        loadData();
    }, 15000);

});