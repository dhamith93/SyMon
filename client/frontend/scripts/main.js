document.addEventListener('DOMContentLoaded', ()=> {
    axios.defaults.headers.post['Accept-Encoding'] = 'gzip';
    loadAgents = () => {
        axios.get('/agents').then((response) => {
            try {
                servers = response.data.Data.AgentIDs                
                handleAgents(response.data.Data.AgentIDs);
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

    loadAgents();
})