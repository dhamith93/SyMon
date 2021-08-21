document.addEventListener('DOMContentLoaded', ()=> {
    const systemTable = document.getElementById('system-table');
    const cpuTable = document.getElementById('cpu-table');
    const memoryTable = document.getElementById('memory-table');
    const swapTable = document.getElementById('swap-table');
    const cpuUsageTable = document.getElementById('cpu-usage-table');
    const memoryUsageTable = document.getElementById('memory-usage-table');
    const agentsUl = document.getElementById('dropdown1');
    const procHeaders = ['User', 'PID', 'CPU %', 'Memory %', 'Command'];
    let serverTime = 0;
    let hourBefore = 0;
    let serverId = '';

    const elems = document.querySelectorAll('.dropdown-trigger');
    const instances = M.Dropdown.init(elems, null);    

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
        populateTable(systemTable, data);
        loadCPUUsage();
        loadMemoryUsage();
        loadCPU();
        loadMemory();
        loadSwap();
        loadProcessCPUUsage();
        loadProcessMemUsage();
    }
    loadAgents();    
    loadSysInfo();
    setInterval(() => {
        loadSysInfo();
    }, 60000);
});