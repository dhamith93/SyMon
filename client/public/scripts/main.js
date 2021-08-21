document.addEventListener('DOMContentLoaded', ()=> {
    const systemTable = document.getElementById('system-table');
    const cpuTable = document.getElementById('cpu-table');
    const memoryTable = document.getElementById('memory-table');
    const swapTable = document.getElementById('swap-table');
    const cpuUsageTable = document.getElementById('cpu-usage-table');
    const memoryUsageTable = document.getElementById('memory-usage-table');
    const checkBoxes = document.querySelectorAll(".metric-check-boxes");
    const agentsUl = document.getElementById('dropdown1');
    const procHeaders = ['User', 'PID', 'CPU %', 'Memory %', 'Command'];
    let serverTime = 0;
    let hourBefore = 0;
    let serverId = '';
    let systemEnabled = true;
    let cpuEnabled = true;
    let usageGraphEnabled = true;
    let memoryEnabled = true;
    let swapEnabled = true;
    let procCpuEnabled = true;
    let procMemEnabled = true;

    const elems = document.querySelectorAll('.dropdown-trigger');
    const instances = M.Dropdown.init(elems, null);
    
    checkBoxes.forEach(checkBox => {
        checkBox.addEventListener('change', e => {
            let id = e.target.id; 
            switch (id) {
                case 'system':
                    systemEnabled = e.target.checked;
                    break;
                case 'cpu':
                    cpuEnabled = e.target.checked;
                    break;
                case 'mem':
                    memoryEnabled = e.target.checked;
                    break;
                case 'swap':
                    swapEnabled = e.target.checked;
                    break;
                case 'proc-cpu':
                    procCpuEnabled = e.target.checked;
                    break;
                case 'proc-mem':
                    procMemEnabled = e.target.checked;
                    break;
                default:
                    break;
            }
            if (!e.target.checked) {
                document.getElementById(id + '-div').style.display = 'none';
            } else {
                document.getElementById(id + '-div').style.display = 'block';
            }
        });
    });

    let handleAgents = (agents) => {
        agents.forEach(agent => {
            let a = document.createElement('a');
            let li = document.createElement('li');
            a.setAttribute('href', '#'+agent);
            a.appendChild(document.createTextNode(agent));
            li.appendChild(a);
            li.addEventListener('click', e => {
                serverId = agent;
                loadSysInfo();
            });
            agentsUl.appendChild(li);
        });
    }

    let loadAgents = () => {
        axios.get('/agents').then((response) => {
            try {
                handleAgents(response.data.Data.AgentIDs);
            } catch (e) {
                console.error(e);
            }
        }, (error) => {
            console.error(error);
        }); 
    }

    let loadSysInfo = () => {
        if (serverId) {
            axios.get('/system?serverId='+serverId).then((response) => {
                handleResponse(response.data.Data);
            }, (error) => {
                console.error(error);
            }); 
        }
    }

    let loadCPU = () => {
        axios.get('/proc?serverId='+serverId).then((response) => {
            populateTable(cpuTable, response.data.Data);
        }, (error) => {
            console.error(error);
        }); 
    }

    let loadMemory = () => {
        axios.get('/memory?serverId='+serverId).then((response) => {
            populateTable(memoryTable, response.data.Data);
        }, (error) => {
            console.error(error);
        }); 
    }

    let loadSwap = () => {
        axios.get('/swap?serverId='+serverId).then((response) => {
            populateTable(swapTable, response.data.Data);
        }, (error) => {
            console.error(error);
        }); 
    }

    let loadCPUUsage = () => {
        axios.get('/processor-usage-historical?serverId='+serverId+'&from='+hourBefore+'&to='+serverTime).then((response) => {            
            console.log(response.data.Data)
        }, (error) => {
            console.error(error);
        }); 
    }

    let loadMemoryUsage = () => {
        axios.get('/memory-historical?serverId='+serverId+'&from='+hourBefore+'&to='+serverTime).then((response) => {            
            console.log(response.data.Data)
        }, (error) => {
            console.error(error);
        }); 
    }

    let loadProcessCPUUsage = () => {
        axios.get('/cpuusage?serverId='+serverId).then((response) => {
            handleUsage(response.data.Data, cpuUsageTable)
        }, (error) => {
            console.error(error);
        }); 
    }

    let loadProcessMemUsage = () => {
        axios.get('/memusage?serverId='+serverId).then((response) => {            
            handleUsage(response.data.Data, memoryUsageTable)
        }, (error) => {
            console.error(error);
        }); 
    }

    let clearTable = async(table) => {
        while (table.firstChild) {
            table.removeChild(table.lastChild);
        }
    }

    let populateTable = (table, data) => {
        if (data) {
            clearTable(table).then(() => {
                for (let key in data) {
                    if (data.hasOwnProperty(key) && key !== 'Time') {
                        let row = table.insertRow(-1);
                        let cell1 = row.insertCell(-1);
                        cell1.innerHTML = key;
                        let cell2 = row.insertCell(-1);
                        cell2.innerHTML = data[key];
                    }
                }
            });
        }
    }

    let handleUsage = (usage, table) => {
        if (usage) {
            clearTable(table).then(() => {
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
                    let data = [row['User'], row['PID'], row['CPUUsage'], row['MemoryUsage'], row['Command']];
                    data.forEach(i => {
                        let cell = tableRow.insertCell(-1);
                        cell.innerHTML = i;
                    });
                });
            });
        }
    }

    let handleResponse = (data) => {
        serverTime = data['Time'];
        hourBefore = serverTime - 3600;
        if (systemEnabled)
            populateTable(systemTable, data);

        if (usageGraphEnabled)
            loadCPUUsage();
        
        if (usageGraphEnabled)
            loadMemoryUsage();

        if (cpuEnabled)
            loadCPU();

        if (memoryEnabled)
            loadMemory();

        if (swapEnabled)
            loadSwap();
        
        if (procCpuEnabled)
            loadProcessCPUUsage();

        if (procMemEnabled)
            loadProcessMemUsage();
    }
    loadAgents();    
    loadSysInfo();
    setInterval(() => {
        loadSysInfo();
    }, 60000);
});