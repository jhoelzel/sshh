import { createRoot } from 'react-dom/client'
import '@xterm/xterm/css/xterm.css'
import './styles/app.css'
import { Bootstrap } from './app/Bootstrap'

const root = document.getElementById('root')

if (!root) {
  throw new Error('application root is missing')
}

createRoot(root).render(<Bootstrap />)
