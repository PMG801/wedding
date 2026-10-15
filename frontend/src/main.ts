import { mount } from 'svelte'
import '@fontsource/cormorant-garamond/latin-500.css'
import App from './App.svelte'
import './app.css'

mount(App, { target: document.getElementById('app')! })
