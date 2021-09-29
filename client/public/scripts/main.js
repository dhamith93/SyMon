document.addEventListener('DOMContentLoaded', ()=> {
    const CPU_COLOR = '#FF5733';
    const MEM_COLOR = '#5AD6A9';
    const NET_RX_COLOR = '#FF8033';
    const NET_TX_COLOR = '#33AFFF';
    const getBtn = document.getElementById('get-btn');
    const resetBtn = document.getElementById('reset-btn');
    const systemTable = document.getElementById('system-table');
    const cpuTable = document.getElementById('cpu-table');
    const memoryTable = document.getElementById('memory-table');
    const swapTable = document.getElementById('swap-table');
    const cpuUsageTable = document.getElementById('cpu-usage-table');
    const memoryUsageTable = document.getElementById('memory-usage-table');
    const servicesTable = document.getElementById('services-table');
    const checkBoxes = document.querySelectorAll('.metric-check-boxes');
    const agentsUl = document.getElementById('dropdown-server');
    const customMetricsDiv = document.getElementById('custom-metrics');
    const customMetricsDisplayArea = document.getElementById('custom-metrics-display-area');
    const procHeaders = ['PID', 'CPU %', 'Memory %', 'Command'];
    let timeZone = '';
    let toTime = 0;
    let fromTime = 0;
    let serverId = '';
    let loadingFromCustomRange = false;
    let loadingPoinInTime = false;
    let systemEnabled = true;
    let usageGraphEnabled = true;
    let memoryEnabled = true;
    let swapEnabled = true;
    let disksEnabled = true;
    let servicesEnabled = true;
    let networksEnabled = true;
    let customMetricsEnabled = true;
    let customMetricsLoaded = false;
    let enabledCustomMetrics = [];
    let customMetricCharts = [];
    let procCpuEnabled = true;
    let procMemEnabled = true;
    let isCPUFirstTime = true;
    let isMemFirstTime = true;
    let isNetworkFirstTime = true;
    let cpuChart = null;
    let memChart = null;
    let networkChart = null;
    let orgRx = 0;
    let orgTx = 0;
    let currentActiveNavLi = null;

    const elems = document.querySelectorAll('.sidenav');
    const instances = M.Sidenav.init(elems);

    document.getElementById('loader').style.display = 'none';
    document.getElementsByTagName('main')[0].style.display = 'block';

    axios.defaults.headers.post['Accept-Encoding'] = 'gzip';

    function toggleMetricsSwitch(id, isChecked) {
        switch (id) {
            case 'system':
                systemEnabled = isChecked;
                break;
            case 'mem':
                memoryEnabled = isChecked;
                break;
            case 'swap':
                swapEnabled = isChecked;
                break;
            case 'disk':
                disksEnabled = isChecked;
                break;
            case 'network':
                networksEnabled = isChecked;
                break;
            case 'proc-cpu':
                procCpuEnabled = isChecked;
                break;
            case 'proc-mem':
                procMemEnabled = isChecked;
                break;
            case 'custom-metric':
                customMetricsEnabled = isChecked;
                break;
            default:
                break;
        }
        if (!isChecked) {
            document.getElementById(id + '-div').style.display = 'none';
        } else {
            document.getElementById(id + '-div').style.display = 'block';
        }
    }
    
    checkBoxes.forEach(checkBox => {
        document.getElementById(checkBox.id + '-div').style.display = 'none';
        checkBox.addEventListener('change', e => {
            let id = e.target.id; 
            toggleMetricsSwitch(id, e.target.checked);
        });
    });

    function handleAgents(agents) {
        agents.forEach(agent => {
            let a = document.createElement('a');
            let li = document.createElement('li');
            a.setAttribute('href', '#'+agent);
            a.appendChild(document.createTextNode(agent));
            li.appendChild(a);
            li.addEventListener('click', e => {
                if (currentActiveNavLi !== null) {
                   currentActiveNavLi.classList.remove('active');
                }
                currentActiveNavLi = e.target.parentNode;
                currentActiveNavLi.classList.add('active');

                clearElement(customMetricsDisplayArea);
                clearElement(customMetricsDiv);
                customMetricCharts = [];
                enabledCustomMetrics = [];
                customMetricsLoaded = false;

                serverId = agent;
                checkBoxes.forEach(c => {
                    toggleMetricsSwitch(c.id, c.checked);
                });
                if (cpuChart !== null) {
                    cpuChart.destroy();
                }
                if (memChart !== null) {
                    memChart.destroy();
                }
                if (networkChart !== null) {
                    networkChart.destroy();
                }
                isCPUFirstTime = true;
                isMemFirstTime = true;
                isNetworkFirstTime = true;
                document.getElementById('date-range-row').style.display = 'block';
                loadSysInfo();
            });
            agentsUl.appendChild(li);
        });
    }

    function loadAgents() {
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

    function loadSysInfo() {
        if (serverId) {
            axios.get('/system?serverId='+serverId).then((response) => {
                handleResponse(response.data.Data);
            }, (error) => {
                console.error(error);
            }); 
        }
    }

    function loadCPU(time = null) {
        let url = '/proc?serverId='+serverId;
        if (time !== null) {
            url = url + '&time=' + time;
        }
        axios.get(url).then((response) => {
            populateTable(cpuTable, response.data.Data);
        }, (error) => {
            console.error(error);
        }); 
    }

    function loadMemory(time = null) {
        let url = '/memory?serverId='+serverId;
        if (time !== null) {
            url = url + '&time=' + time;
        }
        axios.get(url).then((response) => {
            let data = {
                'Total': response.data.Data[2],
                'Used': response.data.Data[3],
                'Free': response.data.Data[4],
                'Used %': response.data.Data[1],
            };
            populateTable(memoryTable, data);
        }, (error) => {
            console.error(error);
        }); 
    }

    function loadSwap(time = null) {
        let url = '/swap?serverId='+serverId;
        if (time !== null) {
            url = url + '&time=' + time;
        }
        axios.get(url).then((response) => {
            let data = {
                'Total': response.data.Data[2],
                'Used': response.data.Data[3],
                'Free': response.data.Data[4],
                'Used %': response.data.Data[1],
            };
            populateTable(swapTable, data);
        }, (error) => {
            console.error(error);
        }); 
    }

    function dataPointClickHandler(e, el) {
        if (el.length > 0) {
            try {
                let time = e.chart.data.labels[el[0].index].getTime() / 1000;
                axios.get('/system?serverId='+serverId+'&time='+time).then((response) => {
                    loadingPoinInTime = true;
                    populateTable(systemTable, response.data.Data);
                    loadInTime(time);
                }, (error) => {
                    console.error(error);
                });
            } catch (e) {
                console.log(e);
            }
        }
    }

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

    document.getElementById('net-chart-reset').addEventListener('click', () => {
        if (networkChart !== null) {
            networkChart.resetZoom();
        }
    });

    function handleDisks(data) {
        let parentDiv = document.getElementById('disks');
    
        clearElement(parentDiv).then(() => {
            data.Disks.forEach(disk => {
                let cardDiv = document.createElement('div');
                cardDiv.style.margin = '20px';
                let cardContentDiv = document.createElement('div');
                let table = document.createElement('table');
                cardDiv.classList.add('card');
        
                let canvas = document.createElement('canvas');
                cardDiv.appendChild(canvas);
        
                let tbody = document.createElement('tbody');
                let processedDisk = {
                    'File system' : disk[0],
                    'Mount point' : disk[1],
                    'Type' : disk[2],
                    'Size' : disk[3],
                    'Free' : disk[4],
                    'Used' : disk[5],
                    'Used %' : disk[6],
                    'Inodes' : disk[7],
                    'Inodes free' : disk[8],
                    'Inodes used' : disk[9],
                    'Inodes used %' : disk[10],
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
        
                let diskData = convertToSame(disk[4], disk[5]);
                let units = getUnits(disk[4], disk[5]);
        
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
                        plugins: {
                            tooltip: {
                                callbacks: {
                                    label: (context) => {
                                        return context.formattedValue + units[context.dataIndex];
                                    }
                                }
                            }
                        },
                        responsive: false
                    }
                });
                
                parentDiv.appendChild(cardDiv);
            });
        });
    }

    function loadDisks(time = null) {
        let url = '/disks?serverId='+serverId;
        if (time !== null) {
            url = url + '&time=' + time;
        }
        axios.get(url).then((response) => {
            handleDisks(response.data.Data)
        }, (error) => {
            console.error(error);
        }); 
    }

    function handleNetworks(data) {
        let parentDiv = document.getElementById('networks');
    
        clearElement(parentDiv).then(() => {
            data.forEach(network => {
                let cardDiv = document.createElement('div');
                cardDiv.style.margin = '20px';
                let cardContentDiv = document.createElement('div');
                let table = document.createElement('table');
                cardDiv.classList.add('card');
        
                let tbody = document.createElement('tbody');
                let processedNetwork = {
                    'IP' : network[0],
                    'Interface' : network[1],
                    'Rx' : convertTo(network[2], 'B', 'M') + 'M',
                    'Tx' : convertTo(network[3], 'B', 'M') + 'M',
                }
                
                for (let key in processedNetwork) {
                    let tr = document.createElement('tr');
                    let td1 = document.createElement('td');
                    td1.classList.add('strong-td');
                    td1.appendChild(document.createTextNode(key));
                    let td2 = document.createElement('td');
                    let value = processedNetwork[key];
                    td2.appendChild(document.createTextNode(value));
                    tr.appendChild(td1);
                    tr.appendChild(td2);
                    tbody.appendChild(tr);
                }
        
                table.appendChild(tbody);
                cardContentDiv.appendChild(table)
                cardDiv.appendChild(cardContentDiv);
                parentDiv.appendChild(cardDiv);
            });
        });
    }

    function loadNetworks(time = null) {
        let url = '/network?serverId='+serverId;
        if (time !== null) {
            url = url + '&time=' + time;
        }
        axios.get(url).then((response) => {
            handleNetworks(response.data.Data[0].Networks)
            if (!isNetworkFirstTime && !loadingPoinInTime && networkChart !== null) {
                updateNetworkChart(networkChart, response.data.Data[0]);
                return;
            }
        }, (error) => {
            console.error(error);
        }); 
    }

    function loadNetworksBandwidth() {
        let url = '/network?serverId='+serverId+'&from='+fromTime+'&to='+toTime;
        axios.get(url).then((response) => {
            let data = response.data.Data;
            let oldest = data.pop().Networks;
            orgRx = parseInt(oldest[0][2], 10);
            orgTx = parseInt(oldest[0][3], 10);
            let processedDataRx = [];
            let processedDataTx = [];
            let labels = [];
            data.forEach(row => {
                let iface = row.Networks[0];
                let newRx = parseInt(iface[2], 10);
                let newTx = parseInt(iface[3], 10);
                let diffRateRx = (newRx - orgRx) / 60;
                let diffRateTx = (newTx - orgTx) / 60;
                orgRx = newRx;
                orgTx = newTx;
                labels.push(new Date(row['Time'] * 1000));
                processedDataRx.push(convertTo(diffRateRx, 'B', 'K'));
                processedDataTx.push(convertTo(diffRateTx, 'B', 'K'));
            });

            showNetworkUsageHistory(processedDataRx, processedDataTx, labels, oldest[0]['IP']);

        }, (error) => {
            console.error(error);
        });
    }

    function showNetworkUsageHistory(processedDataRx, processedDataTx, labels, title) {
        let ctx = document.getElementById('network-usage').getContext('2d');    
        let cData = [];
        let cOptions = [];
    
        cData = {
            labels: labels,
            datasets: [{
                label: 'RX',
                borderColor: NET_RX_COLOR,
                data: processedDataRx
            },
            {
                label: 'TX',
                borderColor: NET_TX_COLOR,
                data: processedDataTx
            }
        ],
        };
        cOptions = {
            title: {
                display: true,
                text: title
            },
            animation: {
                duration: 0
            },
            scales: {
                y: {
                    display: true,
                    min: 0,
                    max: Math.max(Math.max(...processedDataRx), Math.max(...processedDataTx)) + 100,
                    title: {
                        display: true,
                        text: 'kB/s'
                    }
                },
                x: {
                    type: 'timeseries',
                    ticks:{
                        display: true
                    }
                }
            },
            plugins: {
                tooltip: {
                    callbacks: {
                        label: (context) => {
                            return context.parsed.y + 'kB/s';
                        }
                    }
                },
                zoom: {
                    limits: {
                        x: {min: 0, max: 'original'}
                    },
                    pan: {
                        enabled: true
                    },
                    zoom: {
                        wheel: {
                            enabled: true,
                            modifierKey: 'ctrl'
                        },
                        pinch: {
                            enabled: true
                        },
                        mode: 'xy',
                    }
                }
            },
            onClick: (e, el) => {
                dataPointClickHandler(e, el);
            },
            maintainAspectRatio: true
        };
    
        if (networkChart !== null) {
            networkChart.destroy();
        }
        networkChart = new Chart(ctx, {
            type: 'line',
            data: cData,
            options: cOptions
        });
        isNetworkFirstTime = false;
    }

    function updateNetworkChart(chart, data) {
        if (data && chart != null) {
            let newRx = parseInt(data.Networks[0][2], 10);
            let newTx = parseInt(data.Networks[0][3], 10);
            let diffRateRx = (newRx - orgRx) / 60;
            let diffRateTx = (newTx - orgTx) / 60;
            orgRx = newRx;
            orgTx = newTx;
            chart.data.datasets[0].data.shift()
            chart.data.datasets[1].data.shift()
            chart.data.labels.shift();
            chart.data.datasets[0].data.push(convertTo(diffRateRx, 'B', 'K'));
            chart.data.datasets[1].data.push(convertTo(diffRateTx, 'B', 'K'));
            chart.data.labels.push(new Date(data['Time'] * 1000));
            chart.options.scales.y.max = Math.max(Math.max(...chart.data.datasets[0].data), Math.max(...chart.data.datasets[1].data)) + 100;
            chart.update();
        }
    }

    function loadServices(time = null) {
        let url = '/services?serverId='+serverId;
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

    function loadCPUUsage() {
        let url = '/processor-usage-historical?serverId='+serverId+'&from='+fromTime+'&to='+toTime;
        if (!isCPUFirstTime) {
            url = '/processor-usage-historical?serverId='+serverId;
        }
        axios.get(url).then((response) => {
            let processedData = processHistoricalData(response.data.Data);    
            if (!isCPUFirstTime && cpuChart !== null) {
                updateChart(cpuChart, processedData['labels'], processedData['data']);
                return;
            }
            isCPUFirstTime = false;
            if (cpuChart !== null) cpuChart.destroy();
            cpuChart = generateUsageChart(processedData, document.getElementById('cpu-usage'), 'CPU', CPU_COLOR, context => context.parsed.y + '%', dataPointClickHandler);
        }, (error) => {
            console.error(error);
        }); 
    }

    function loadMemoryUsage() {
        let url = '/memory-historical?serverId='+serverId+'&from='+fromTime+'&to='+toTime;
        if (!isMemFirstTime) {
            url = '/memory-historical?serverId='+serverId;
        }
        axios.get(url).then((response) => {            
            let processedData = processHistoricalData(response.data.Data);    
            if (!isMemFirstTime && memChart !== null) {
                updateChart(memChart, processedData['labels'], processedData['data']);
                return;
            }
            isMemFirstTime = false;
            if (memChart !== null) memChart.destroy();
            memChart = generateUsageChart(processedData, document.getElementById('mem-usage'), 'MEM', MEM_COLOR, context => context.parsed.y + '%', dataPointClickHandler);
        }, (error) => {
            console.error(error);
        }); 
    }

    function loadProcesses(time = null) {
        let url = '/processes?serverId='+serverId;
        if (time !== null) {
            url = url + '&time=' + time;
        }
        axios.get(url).then((response) => {
            if (procCpuEnabled)
                handleUsage(response.data.Data.CPU, cpuUsageTable);

            if (procMemEnabled)
                handleUsage(response.data.Data.Memory, memoryUsageTable);
        }, (error) => {
            console.error(error);
        }); 
    }

    function loadCustomMetrics() {
        clearElement(customMetricsDisplayArea);
        customMetricCharts = [];
        enabledCustomMetrics.forEach(metric => {
            let url = '/custom?serverId='+serverId+'&from='+fromTime+'&to='+toTime+'&custom-metric='+metric;
            axios.get(url).then((response) => {
                if (response.data.Data) {
                    let processedData = processHistoricalData(response.data.Data, 'custom');
                    let divId = 'custom-data-for-' + metric;
                    let canvas = document.createElement('canvas');
                    canvas.setAttribute('width', '800px');
                    canvas.setAttribute('id', divId);
                    customMetricsDisplayArea.appendChild(canvas);
                    customMetricCharts.push(generateUsageChart(processedData, canvas, metric, CPU_COLOR, context => context.parsed.y + ' ' + response.data.Data[0].Unit));
                }
            }, (error) => {
                console.error(error);
            })
        });
    }

    function loadCustomMetricNames() {
        axios.get('/custom-metric-names?serverId='+serverId).then((response) => {
            try {
                if (response.data.Data) {
                    let data = response.data.Data;
                    if (data.CustomMetrics) {
                        clearElement(customMetricsDisplayArea);
                        data.CustomMetrics.forEach(metric => {
                            let div = document.createElement('div');
                            div.classList.add('switch');
                            let label = document.createElement('label');
                            let span = document.createElement('span');
                            span.classList.add('lever');
                            let chkbox = document.createElement('input');
                            chkbox.setAttribute('type', 'checkbox');
                            chkbox.classList.add('metric-check-boxes')
                            chkbox.addEventListener('click', e => {
                                enabledCustomMetrics = enabledCustomMetrics.filter(item => item !== metric);
                                if (e.target.checked) {
                                    enabledCustomMetrics.push(metric);
                                }
                                loadCustomMetrics();
                            });
                            label.appendChild(chkbox);
                            label.appendChild(span);
                            label.appendChild(document.createTextNode(metric));
                            div.appendChild(label);
                            customMetricsDiv.appendChild(div);
                        });
                    }
                    customMetricsLoaded = true;
                }
            } catch (e) { }
        }, (error) => {
            console.error(error);
        }); 
    }

    function handleUsage (usage, table) {
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

    getBtn.addEventListener('click', e => {
        let from = moment(document.getElementById('from-datetime').value);
        let to = moment(document.getElementById('to-datetime').value);
        if (from > to) {
            return;
        }
        loadingFromCustomRange = true;
        fromTime = from.unix();
        toTime = to.unix();

        isCPUFirstTime = true;
        isMemFirstTime = true;
        isNetworkFirstTime = true;

        let tmpFromTime = toTime - 60;
        axios.get('/system?serverId='+serverId+'&from='+tmpFromTime+'&to='+toTime).then((response) => {
            let data = response.data.Data;
            populateTable(systemTable, data);
            loadInTime(data.Time);
        }, (error) => {
            console.error(error);
        });

        if (usageGraphEnabled) {
            loadCPUUsage();
            loadMemoryUsage();
            loadNetworksBandwidth();
        }

        if (customMetricsEnabled && !customMetricsLoaded) {
            loadCustomMetricNames();
        }

        if (customMetricsEnabled) {
            loadCustomMetrics();
        }
    });

    resetBtn.addEventListener('click', e => {
        loadingFromCustomRange = false;
        isCPUFirstTime = true;
        isMemFirstTime = true;
        isNetworkFirstTime = true;
        isFirstRodeo = true;
        loadingPoinInTime = false;
        loadSysInfo();
    });

    function loadInTime(time = null) {
        if (memoryEnabled)
            loadMemory(time);

        if (swapEnabled)
            loadSwap(time);

        if (disksEnabled)
            loadDisks(time);

        if (networksEnabled)
            loadNetworks(time);

        if (servicesEnabled)
            loadServices(time);

        if (procCpuEnabled || procMemEnabled)
            loadProcesses(time);
    }

    function handleResponse(data) {
        toTime = data['Time'];
        fromTime = toTime - 3600;
        timeZone = data.TimeZone;

        moment.tz.setDefault(timeZone);

        flatpickr('#from-datetime', {
            enableTime: true,
            dateFormat: "Y-m-d H:i",
            defaultDate: new Date(fromTime * 1000),
            maxDate: new Date(toTime * 1000),
            formatDate: (date, format, locale) => { return moment(date).format('YYYY-MM-DD HH:mm:ss'); }
        });
    
        flatpickr('#to-datetime', {
            enableTime: true,
            dateFormat: "Y-m-d H:i",
            defaultDate: new Date(toTime * 1000),
            maxDate: new Date(toTime * 1000),
            formatDate: (date, format, locale) => { return moment(date).format('YYYY-MM-DD HH:mm:ss'); }
        });

        if (systemEnabled)
            populateTable(systemTable, data);

        if (usageGraphEnabled) {
            loadCPUUsage();
            loadMemoryUsage();
        }

        loadInTime();

        if (networksEnabled && isNetworkFirstTime)
            loadNetworksBandwidth();

        if (customMetricsEnabled && !customMetricsLoaded) {
            loadCustomMetricNames();
        }

        if (customMetricsEnabled) {
            loadCustomMetrics();
        }
    }
    loadAgents();    
    loadSysInfo();
    setInterval(() => {
        if (!loadingFromCustomRange && !loadingPoinInTime)
            loadSysInfo();
    }, 60000);
});