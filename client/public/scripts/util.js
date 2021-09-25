let getUnits = (free, used) => {
    let out = [];
    
    if (free.length > 1 && used.length > 2) {
        let freeUnit = free.substring(free.length - 1,  free.length);
        let usedUnit = used.substring(used.length - 1,  used.length);
        out.push(freeUnit, usedUnit);
    }

    return out;
}

let convertToSame = (free, used) => {
    let out = [];
    
    if (free.length > 1 && used.length > 2) {
        let freeUnit = free.substring(free.length - 1,  free.length);
        let usedUnit = used.substring(used.length - 1,  used.length);
        let freeAmount = parseFloat(free.substring(0,  free.length - 1));
        let usedAmount = parseFloat(used.substring(0,  used.length - 1));

        if (freeUnit !== usedUnit) {
            usedAmount = convertTo(usedAmount, usedUnit, freeUnit);
        }        
        out.push(freeAmount, usedAmount);
    }

    return out;
}

let convertTo = (amount, unit, outUnit) => {
    let out = null;
    switch (unit) {
        case 'B':
            if (outUnit === 'M') {
                out = (amount / 1024) / 1024;
            } else if (outUnit === 'K') {
                out = amount / 1024;
            }
            break;
        case 'M':
            if (outUnit === 'G') {
                out = amount / 1024;
            } else if (outUnit === 'T') {
                out = (amount / 1024) / 1024;
            }
            break;
        case 'M':
            if (outUnit === 'G') {
                out = amount / 1024;
            } else if (outUnit === 'T') {
                out = (amount / 1024) / 1024;
            }
            break;
        case 'G':
            if (outUnit === 'M') {
                out = amount * 1024;
            } else if (outUnit === 'T') {
                out = amount / 1024;
            }
            break;
        case 'T':
            if (outUnit === 'M') {
                out = (amount * 1024) * 1024;
            } else if (outUnit === 'G') {
                out = amount * 1024;
            }
            break;
    }
    return Math.round(out);
}

async function clearElement(element){
    while (element.firstChild) {
        element.removeChild(element.lastChild);
    }
}

function populateTable(table, data) {
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

function processHistoricalData(data, type = 'default') {
    let output = [];
    let usage = [];
    let labels = [];

    data.reverse();

    data.forEach(record => {
        let usageData = null;
        switch (type) {
            case 'custom':
                labels.push(new Date(record['Time'] * 1000));
                usageData = record['Value'];
                break;
            default:
                labels.push(new Date(record[0] * 1000));
                usageData = record[1].replace('%', '');
                break;
        }
        usage.push(parseFloat(usageData));
    });

    output['data'] = usage; 
    output['labels'] = labels;
    return output;
}

function generateUsageChart(processedData, elem, label, color, callback, dataPointClickHandler) {
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
                    max: 100
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
                        label: callback
                    }
                },
                zoom: {
                    limits: {
                        x: {min: 0, max: 'original'}
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
            onClick: dataPointClickHandler,
            maintainAspectRatio: true
        }
    });
}

function updateChart(chart, labels, data) {
    if (chart.data.labels[0].getTime() === labels[0].getTime()) {
        return;
    }
    chart.data.labels.pop();
    chart.data.datasets[0].data.pop();
    chart.data.labels = labels.concat(chart.data.labels);
    chart.data.datasets[0].data = data.concat(chart.data.datasets[0].data);
    chart.update();
}