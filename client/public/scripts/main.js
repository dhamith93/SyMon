document.addEventListener('DOMContentLoaded', ()=> {
    const CPU_COLOR = '#FF5733';
    const MEM_COLOR = '#5AD6A9';
    const systemTable = document.getElementById('system-table');
    const cpuTable = document.getElementById('cpu-table');
    const memoryTable = document.getElementById('memory-table');
    const swapTable = document.getElementById('swap-table');
    const cpuUsageTable = document.getElementById('cpu-usage-table');
    const memoryUsageTable = document.getElementById('memory-usage-table');
    const servicesTable = document.getElementById('services-table');
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
    let disksEnabled = true;
    let servicesEnabled = true;
    let networksEnabled = true;
    let procCpuEnabled = true;
    let procMemEnabled = true;
    let isCPUFirstTime = true;
    let isMemFirstTime = true;
    let cpuChart = null;
    let memChart = null;

    const elems = document.querySelectorAll('.dropdown-trigger');
    const instances = M.Dropdown.init(elems, null);

    document.getElementById('loader').style.display = 'none';
    document.querySelectorAll('.main')[0].style.display = 'unset';

    axios.defaults.headers.post['Accept-Encoding'] = 'gzip'
    
    checkBoxes.forEach(checkBox => {
        document.getElementById(checkBox.id + '-div').style.display = 'none';
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
                case 'disk':
                    disksEnabled = e.target.checked;
                    break;
                case 'network':
                    networksEnabled = e.target.checked;
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
                checkBoxes.forEach(c => {
                    c.checked = true;
                    document.getElementById(c.id + '-div').style.display = 'block';
                });
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

    function processHistoricalData(data, type) {
        let output = [];
        let usage = [];
        let labels = [];
    
        data.reverse();
    
        data.forEach(record => {
            let usageData = null;
            switch (type) {
                case 'cpu-usage':
                    usageData = record['LoadAvg'].replace('%', '');
                    break;
                case 'mem-usage':
                case 'disk':
                    usageData = record['PercentageUsed'].replace('%', '');
                    break;
                default:
                    usageData = -1;
                    break;
            }
            usage.push(parseFloat(usageData));
            labels.push(new Date(record['Time'] * 1000));
        });
    
        output['data'] = usage; 
        output['labels'] = labels;
        return output;
    }

    let showUsageHistory = (data, elemId, label, type, callback = null) => {
        let processedData = processHistoricalData(data, type);
    
        if (!isCPUFirstTime && type === 'cpu-usage' && cpuChart !== null) {
            updateChart(cpuChart, processedData['labels'], processedData['data'], label);
            return;
        }
        
        if (!isMemFirstTime && type === 'mem-usage' && memChart !== null) {
            updateChart(memChart, processedData['labels'], processedData['data'], label);
            return;
        }
    
        if (type === 'cpu-usage')
            isCPUFirstTime = false;
        if (type === 'mem-usage')
            isMemFirstTime = false;
    
        let ctx = document.getElementById(elemId).getContext('2d');
        let last = processedData['data'][processedData['data'].length - 1];
    
        let cData = [];
        let cOptions = [];
    
        cData = {
            labels: processedData['labels'],
            datasets: [{
                label: label + ': ' + last + '%',
                borderColor: (type === 'cpu-usage') ? CPU_COLOR : MEM_COLOR,
                data: processedData['data']
            }],
        };
        cOptions = {
            animation: {
                duration: 0
            },
            scales: {
                yAxes: [{
                    ticks: {
                        min: 0,
                        max: 100,
                        stepSize: 10
                    }
                }],
                xAxes: [{
                    type: 'time',
                    time: {
                        tooltipFormat:'HH:mm:ss MMM D, YYYY',
                        unit: 'minute',
                        displayFormats: {
                            minute: 'HH:mm:ss MMM D'
                        }
                    },
                    ticks:{
                        display: true,
                        autoSkip: true,
                        maxTicksLimit: 11
                    }
                }]
            },
            tooltips: {
                callbacks: {
                    label: function(tooltipItem) {
                        return tooltipItem.yLabel + "%";
                    }
                }
            }
        };
    
        if (type === 'cpu-usage') {
            if (cpuChart !== null) {
                cpuChart.destroy();
            }
            cpuChart = new Chart(ctx, {
                type: 'line',
                data: cData,
                options: cOptions
            });
        } else if (type === 'mem-usage') {
            if (memChart !== null) {
                memChart.destroy();
            }
            memChart = new Chart(ctx, {
                type: 'line',
                data: cData,
                options: cOptions
            });
        }
    
        // document.getElementById(elemId).addEventListener('click', (e) => {
        //     let activePoints = (type === 'cpu-usage') ? 
        //         cpuChart.getElementsAtEvent(e) : memChart.getElementsAtEvent(e);
        //     let firstPoint = activePoints[0];
        //     let unixTime = (type === 'cpu-usage') ?
        //         cpuChart.data.labels[firstPoint._index] : memChart.data.labels[firstPoint._index];;
        //     if (firstPoint !== undefined) {
        //         callback(unixTime);
        //     }
        // });
    }

    let updateChart = (chart, labels, data, label) => {
        if (chart.data.labels[0].getTime() === labels[0].getTime()) {
            return;
        }
    
        let last = data[data.length - 1];
        chart.data.labels.pop();
        chart.data.datasets[0].data.pop();
        chart.data.datasets[0].label = label + ': ' + last + '%'
    
        let newData = [];
        let newLabels = [];
    
        newData = newData.concat(data);
        newLabels = newLabels.concat(labels);
        newData = newData.concat(chart.data.datasets[0].data);
        newLabels = newLabels.concat(chart.data.labels);
    
        chart.data.datasets[0].data = newData;
        chart.data.labels = newLabels;
        chart.update();
    }

    let handleDisks = (data) => {
        let parentDiv = document.getElementById('disks');
    
        clearElement(parentDiv).then(() => {
            data.forEach(disk => {
                let cardDiv = document.createElement('div');
                cardDiv.style.margin = '20px';
                let cardContentDiv = document.createElement('div');
                let table = document.createElement('table');
                cardDiv.classList.add('card');
        
                let canvas = document.createElement('canvas');
                cardDiv.appendChild(canvas);
        
                let tbody = document.createElement('tbody');
                let processedDisk = {
                    'File system' : disk['FileSystem'],
                    'Mount point' : disk['MountPoint'],
                    'Type' : disk['Type'],
                    'Size' : disk['Size'],
                    'Free' : disk['Free'],
                    'Used' : disk['Used'],
                    'Used %' : disk['PercentageUsed'],
                    'Inodes' : disk['Inodes'],
                    'Inodes free' : disk['IFree'],
                    'Inodes used' : disk['IUsed'],
                    'Inodes used %' : disk['IPercentageUsed'],
                }
        
                for (let key in processedDisk) {
                    let tr = document.createElement('tr');
                    let td1 = document.createElement('td');
                    td1.appendChild(document.createTextNode(key));
                    td1.classList.add('strong-td');
                    let td2 = document.createElement('td');
                    td2.appendChild(document.createTextNode(processedDisk[key]));
                    tr.appendChild(td1);
                    tr.appendChild(td2);
                    tbody.appendChild(tr);
                }
        
                table.appendChild(tbody);
                cardContentDiv.appendChild(table)
                cardDiv.appendChild(cardContentDiv);
        
                let diskData = convertToSame(disk['Free'], disk['Used']);
                let units = getUnits(disk['Free'], disk['Used']);
        
                let diskUsageChart = new Chart(canvas, {
                    type: 'doughnut',
                    data: {
                        datasets: [{
                            data: diskData,
                            backgroundColor: ['#0074D9', '#FF4136']
                        }],
                        labels: [
                            'Free',
                            'Used'
                        ]
                    },
                    options : {
                        tooltips: {
                            callbacks: {
                                label: function(tooltipItem, data) {
                                    let index = tooltipItem.index;
                                    return data.datasets[tooltipItem.datasetIndex].data[index] + units[0];
                              },
                              title: function(tooltipItem, data) {
                                return data.datasets[tooltipItem[0].datasetIndex].label;
                              }
                            }
                        },
                        responsive:false
                    }
                });
                
                parentDiv.appendChild(cardDiv);
            });
        });
    }

    let loadDisks = () => {
        axios.get('/disks?serverId='+serverId).then((response) => {
            handleDisks(response.data.Data)
        }, (error) => {
            console.error(error);
        }); 
    }

    let handleNetworks = (data) => {
        let parentDiv = document.getElementById('networks');
    
        clearElement(parentDiv).then(() => {
            data.forEach(network => {
                let cardDiv = document.createElement('div');
                cardDiv.style.margin = '20px';
                let cardContentDiv = document.createElement('div');
                let table = document.createElement('table');
                cardDiv.classList.add('card');
        
                let tbody = document.createElement('tbody');
                
                for (let key in network) {
                    if (key !== 'Time' && network.hasOwnProperty(key)) {
                        let tr = document.createElement('tr');
                        let td1 = document.createElement('td');
                        td1.classList.add('strong-td');
                        td1.appendChild(document.createTextNode(key));
                        let td2 = document.createElement('td');
                        let value = network[key];                        
                        td2.appendChild(document.createTextNode(value));
                        tr.appendChild(td1);
                        tr.appendChild(td2);
                        tbody.appendChild(tr);
                    }
                }
        
                table.appendChild(tbody);
                cardContentDiv.appendChild(table)
                cardDiv.appendChild(cardContentDiv);
                parentDiv.appendChild(cardDiv);
            });
        });
    }

    let loadNetworks = () => {
        axios.get('/network?serverId='+serverId).then((response) => {
            handleNetworks(response.data.Data)
        }, (error) => {
            console.error(error);
        }); 
    }

    let loadServices = () => {
        axios.get('/services?serverId='+serverId).then((response) => {
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

    let loadCPUUsage = () => {
        let url = '/processor-usage-historical?serverId='+serverId+'&from='+hourBefore+'&to='+serverTime;
        if (!isCPUFirstTime) {
            url = '/processor-usage-historical?serverId='+serverId;
        }
        axios.get(url).then((response) => {
            showUsageHistory(response.data.Data, 'cpu-usage', 'CPU', 'cpu-usage');
        }, (error) => {
            console.error(error);
        }); 
    }

    let loadMemoryUsage = () => {
        let url = '/memory-historical?serverId='+serverId+'&from='+hourBefore+'&to='+serverTime;
        if (!isCPUFirstTime) {
            url = '/memory-historical?serverId='+serverId;
        }
        axios.get(url).then((response) => {            
            showUsageHistory(response.data.Data, 'mem-usage', 'MEM', 'mem-usage');
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

    let clearElement = async(element) => {
        while (element.firstChild) {
            element.removeChild(element.lastChild);
        }
    }

    let populateTable = (table, data) => {
        if (data) {
            clearElement(table).then(() => {
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

        if (disksEnabled)
            loadDisks();

        if (networksEnabled)
            loadNetworks();

        if (servicesEnabled)
            loadServices();
        
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