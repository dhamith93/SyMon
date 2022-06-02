document.addEventListener('DOMContentLoaded', ()=> {
    axios.defaults.headers.post['Accept-Encoding'] = 'gzip';
    const STOPPED_COLOR = '#E74C3C';
    const RUNNING_COLOR = '#1ABC9C';
    let servers = [];
    loadAgents = () => {
        axios.get('/agents').then((response) => {
            try {
                servers = response.data.Data.AgentIDs;
                handleAgents(response.data.Data.AgentIDs);
                checkActivity();
            } catch (e) {
                console.error(e);
            }
        }, (error) => {
            console.error(error);
        }); 
    }

    handleAgents = (agents) => {
        agents.forEach(agent => {
            serverDiv = document.createElement('div');
            serverDiv.classList.add('server-item');
            serverStatusDiv = document.createElement('div');
            serverStatusDiv.classList.add('server-status');
            serverStatusDiv.setAttribute('id', agent);
            serverNameDiv = document.createElement('div');
            serverNameDiv.classList.add('server-name');
            serverNameP = document.createElement('p');
            serverNameP.innerHTML = agent;

            serverNameDiv.appendChild(serverNameP);
            serverDiv.appendChild(serverStatusDiv);
            serverDiv.appendChild(serverNameDiv);
            document.getElementById('server-list').appendChild(serverDiv)

            serverDiv.addEventListener('click', e => {
                window.open('server.html?name=' + agent, '_blank');
            });            
        });
    }

    checkActivity = () => {
        servers.forEach(server => {
            axios.get('/isup?serverId='+server).then((response) => {
                try {
                    statusDiv = document.querySelector(`#${server}`);
                    if (response.data.Data.IsUp) {
                        statusDiv.style.backgroundColor = RUNNING_COLOR;
                    } else {
                        statusDiv.style.backgroundColor = STOPPED_COLOR;
                    }
                } catch (e) {
                    console.error(e);
                }
            }, (error) => {
                console.error(error);
            }); 
        });
    }

    loadAgents();
    setInterval(() => {
        checkActivity();
    }, 15000);
})