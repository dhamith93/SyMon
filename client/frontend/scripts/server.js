document.addEventListener('DOMContentLoaded', ()=> {
    const CPU_COLOR = '#FF6B6B';
    const MEM_COLOR = '#5AD6A9';
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
    const cpuCircle = document.querySelector('#cpu-circle');
    const cpuLoadAvgElem = document.querySelector('#cpu-load-avg');
    const memoryCircle = document.querySelector('#memory-circle');
    const memoryLoadElem = document.querySelector('#memory-load');
    const cpuTable = document.querySelector('#cpu-table');
    const memoryTable = document.querySelector('#memory-table');
    const circleStrokeDashOffset = 472;
    const procHeaders = ['PID', 'CPU %', 'Memory %', 'Command'];
    let selectedSection = 'overview-section';
    let firstTime = true;
    let isCPUFirstTime = true;
    let isMemFirstTime = true;
    let cpuChart = null;
    let memChart = null;
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
                if (section.id == link.dataset.section) {
                    section.classList.add('section-active');    
                } else {
                    section.classList.remove('section-active');
                }
            });
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
    
    loadSystem = () => {
        axios.get('/system?serverId='+serverName).then((response) => {
            if (response.data.Status == 'OK') {
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
            if (response.data.Status == 'OK') {
                cpu = response.data.Data[0];
                if (selectedSection == 'overview-section') {
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
            if (response.data.Status == 'OK') {
                memory = response.data.Data;
                // memoryUsage = (memory.PercentageUsed).toFixed(2);
                memory.PercentageUsed = (memory.PercentageUsed).toFixed(2);

                if (selectedSection == 'overview-section') {
                    memoryCircle.style.strokeDashoffset = circleStrokeDashOffset - circleStrokeDashOffset * (memory.PercentageUsed / 100);
                    memoryLoadElem.innerHTML = `${memory.PercentageUsed}%`;
                }
            }
            memory.Usage = `${memory.PercentageUsed}%`;
            memory.Available = `${memory.Available} ${memory.Unit}`;
            memory.Free = `${memory.Free} ${memory.Unit}`;
            memory.Total = `${memory.Total} ${memory.Unit}`;
            memory.Used = `${memory.Used} ${memory.Unit}`;
            delete memory.Unit;
            delete memory.PercentageUsed;
            populateTable(memoryTable, memory);
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
            cpuChart = generateUsageChart(processedData, document.getElementById('cpu-usage-chart'), 'CPU', CPU_COLOR, context => context.parsed.y + '%');
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
            memChart = generateUsageChart(processedData, document.getElementById('memory-usage-chart'), 'MEM', MEM_COLOR, context => context.parsed.y + '%');
        }, (error) => {
            console.error(error);
        }); 
    }

    handleProcessList = (usage, table, tableInSection) => {
        if (usage) {
            if (selectedSection == 'overview-section') {
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

            if (selectedSection == 'cpu-section' || selectedSection == 'memory-section') {
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

    loadData = () => {
        if (selectedSection == 'overview-section') {
            loadMemory();
            loadCPU();
            loadProcesses();
            loadServices();
        }

        if (selectedSection == 'cpu-section') {
            loadProcesses();
            loadCPUUsage();
        }

        if (selectedSection == 'memory-section') {
            loadProcesses();
            loadMemoryUsage()
            loadMemory();
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