Chart.defaults.color = "#fff";

processUsageData = (data, type = 'cpu') => {
    let output = [];
    let usage = [];
    let labels = [];

    data.reverse();
    data.forEach(record => {
        let usageData = null;
        labels.push(new Date(record.Time * 1000));
        switch (type) {
            case 'custom':
                usageData = record.Value;
                break;
            case 'memory':
                usageData = (record.PercentageUsed).toFixed(2)
                break;
            case 'cpu':
                usageData = record.LoadAvg
                break;
        }
        usage.push(parseFloat(usageData));
    });

    output['data'] = usage; 
    output['labels'] = labels;
    return output;
}

processDisksForCharts = (data) => {
    let disks = {};
    let labels = [];
    let diskNames = [];

    data.forEach(record => {
        record.forEach(row => {
            if (!diskNames.includes(row.FileSystem)) {
                diskNames.push(row.FileSystem);
            }
        });
    });

    data.forEach(record => {
        if (record.length > 0) {
            labels.push(new Date(record[0].Time * 1000));
        }
        let time = null;
        let diskNamesCopy = diskNames;
        record.forEach(row => {
            time = new Date(row.Time * 1000)
            diskNames.forEach(disk => {
                if (disk === row.FileSystem) {
                    if (disks[row.FileSystem] === undefined) {
                        disks[row.FileSystem] = [];
                    }
                    disks[row.FileSystem].push({
                        x: time,
                        y: parseFloat(row.Usage.Usage.replace('%', ''))
                    });
                    diskNamesCopy = diskNamesCopy.filter(e => e !== disk);
                }
            });
        });
        diskNamesCopy.forEach(disk => {
            if (disks[disk] === undefined) {
                disks[disk] = [];
            }
            disks[disk].push({
                x: time,
                y: undefined
            });
        });
    });

    let datasets = [];
    labels.reverse();

    Object.entries(disks).forEach(disk => {
        disk[1].reverse();
        datasets.push({
            label: disk[0],
            borderColor: getRandomColor(),
            data: disk[1]
        });
    });

    return {
        labels: labels,
        data: datasets
    };
}

processNetworksForCharts = (data, index = 0, monitorInterval) => {
    let oldest = data.pop();
    let orgRx = parseInt(oldest[index].Usage.RxBytes, 10);
    let orgTx = parseInt(oldest[index].Usage.TxBytes, 10);
    let processedDataRx = [];
    let processedDataTx = [];
    let labels = [];
    data.forEach(row => {
        let newRx = parseInt(row[index].Usage.RxBytes, 10);
        let newTx = parseInt(row[index].Usage.TxBytes, 10);
        let diffRateRx = (newRx - orgRx) / monitorInterval;
        let diffRateTx = (newTx - orgTx) / monitorInterval;
        orgRx = newRx;
        orgTx = newTx;
        labels.push(new Date(row[index].Time * 1000));
        processedDataRx.push(convertTo(diffRateRx, 'B', 'K'));
        processedDataTx.push(convertTo(diffRateTx, 'B', 'K'));
    });
    let cData = {
        labels: labels,
        data: [
            {
                label: 'RX',
                borderColor: getRandomColor(),
                data: processedDataRx
            },
            {
                label: 'TX',
                borderColor: getRandomColor(),
                data: processedDataTx
            }
        ],
    };
    return {
        data: cData, 
        orgRx: orgRx, 
        orgTx: orgTx
    }
}

generateUsageChart = (processedData, elem, label, callback, isMax100 = true) => {
    let options = getOptions(callback);
    if (isMax100)
        options.scales.y.max = 100;
    return new Chart(elem.getContext('2d'), {
        type: 'line',
        data: {
            labels: processedData['labels'],
            datasets: [{
                label: label,
                borderColor: getRandomColor(),
                data: processedData['data']
            }],
        },
        options: options
    });
}

getOptions = (callback) => {
    return {
        animation: {
            duration: 0
        },
        elements: {
            point:{
                radius: 0
            }
        },
        scales: {
            y: {
                display: true,
                min: 0,
                ticks:{
                    color: '#fff'
                }
            },
            x: {
                type: 'timeseries',
                ticks:{
                    display: true,
                    color: '#fff'
                }
            }
        },
        plugins: {
            tooltip: {
                callbacks: {
                    label: callback
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
                    mode: 'x',
                }
            }
        },
        // onClick: dataPointClickHandler,
        maintainAspectRatio: true
    }
}

generateUsageChartForMultiple = (processedData, elem, callback, isMax100 = true) => {
    let options = getOptions(callback);
    if (isMax100)
        options.scales.y.max = 100;
    return new Chart(elem.getContext('2d'), {
        type: 'line',
        data: {
            labels: processedData['labels'],
            datasets: processedData['data'],
        },
        options: options
    });
}

updateChart = (chart, labels, data) => {
    if (chart.data.labels[0].getTime() === labels[0].getTime()) {
        return;
    }
    chart.data.labels.pop();
    chart.data.datasets[0].data.pop();
    chart.data.labels = labels.concat(chart.data.labels);
    chart.data.datasets[0].data = data.concat(chart.data.datasets[0].data);
    chart.update();
}

updateChartForDisks = (chart, data) => {
    let labels = [];
    labels.push(new Date(data[0].Time * 1000));

    if (chart.data.labels[0].getTime() === labels[0].getTime()) {
        return;
    }
    
    chart.data.datasets.forEach(set => {
        let datasetMatched = false;
        let time = null;
        data.forEach(record => {
            if (set.label === record.FileSystem) {
                time = new Date(record.Time * 1000);
                let newData = [{
                    x: time,
                    y: parseFloat(record.Usage.Usage.replace('%', ''))
                }];
                set.data.pop();
                set.data = newData.concat(set.data);
                datasetMatched = true;
            }
        });
        if (!datasetMatched) {
            let newData = [{
                x: time,
                y: undefined
            }];
            set.data.pop();
            set.data = newData.concat(set.data);
        }
    });
    chart.data.labels.pop();
    chart.data.labels = labels.concat(chart.data.labels);
    chart.update();
}

updateChartForNetwork = (chart, data, index = 0, orgRx, orgTx, monitorInterval) => {
    if (data && chart != null) {
        let newRx = parseInt(data[index].Usage.RxBytes, 10);
        let newTx = parseInt(data[index].Usage.TxBytes, 10);
        let diffRateRx = (newRx - orgRx) / monitorInterval;
        let diffRateTx = (newTx - orgTx) / monitorInterval;
        orgRx = newRx;
        orgTx = newTx;
        chart.data.datasets[0].data.shift()
        chart.data.datasets[1].data.shift()
        chart.data.labels.shift();
        chart.data.datasets[0].data.push(convertTo(diffRateRx, 'B', 'K'));
        chart.data.datasets[1].data.push(convertTo(diffRateTx, 'B', 'K'));
        chart.data.labels.push(new Date(data[index].Time * 1000));
        chart.update();
    }
    return [orgRx, orgTx];
}