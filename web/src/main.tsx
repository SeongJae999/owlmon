import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import './api/auth' // axios 인터셉터 등록
import App from './App.tsx'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
