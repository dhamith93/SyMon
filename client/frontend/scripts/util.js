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
            } else if (outUnit === 'G') {
                out = (amount / 1024) / 1024 / 1024;
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

function getRandomColor() {
    var letters = 'BCDEF'.split('');
    var color = '#';
    for (var i = 0; i < 6; i++ ) {
        color += letters[Math.floor(Math.random() * letters.length)];
    }
    return color;
}