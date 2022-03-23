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

generateUsageChart = (processedData, elem, label, color, callback) => {
    return new Chart(elem.getContext('2d'), {
        type: 'line',
        data: {
            labels: processedData['labels'],
            datasets: [{
                label: label,
                borderColor: color,
                data: processedData['data']
            }],
        },
        options: {
            animation: {
                duration: 0
            },
            scales: {
                y: {
                    display: true,
                    min: 0,
                    max: 100,
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
                        mode: 'xy',
                    }
                }
            },
            // onClick: dataPointClickHandler,
            maintainAspectRatio: true
        }
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