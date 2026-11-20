import { mount } from 'svelte';
// latin only, other scripts fall back to the system fonts
import '@fontsource/ibm-plex-sans/latin-400.css';
import '@fontsource/ibm-plex-sans/latin-500.css';
import '@fontsource/ibm-plex-sans/latin-600.css';
import '@fontsource/ibm-plex-mono/latin-400.css';
import '@fontsource/ibm-plex-mono/latin-500.css';
import './app.css';
import App from './App.svelte';

mount(App, { target: document.getElementById('app')! });
