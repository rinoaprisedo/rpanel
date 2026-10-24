import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Toaster } from 'react-hot-toast'
import App from './App.jsx'
import ErrorBoundary from './components/ErrorBoundary.jsx'
import './index.css'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: 1, refetchOnWindowFocus: false },
  },
})

ReactDOM.createRoot(document.getElementById('root')).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <ErrorBoundary>
          <App />
        </ErrorBoundary>
        <Toaster
          position="bottom-right"
          gutter={10}
          toastOptions={{
            duration: 3500,
            style: {
              background: 'var(--toast-bg, #ffffff)',
              color: '#1a1917',
              border: '1px solid #e2e1db',
              borderRadius: '10px',
              boxShadow: '0 4px 20px rgba(0,0,0,.18)',
              padding: '10px 14px',
              fontSize: '13px',
              fontWeight: 500,
              maxWidth: '380px',
            },
            success: {
              iconTheme: { primary: '#ffffff', secondary: '#2d6a4f' },
              style: { background: '#2d6a4f', color: '#ffffff', border: '1px solid #235940' },
            },
            error: {
              iconTheme: { primary: '#ffffff', secondary: '#c0392b' },
              style: { background: '#c0392b', color: '#ffffff', border: '1px solid #a12f22' },
            },
            loading: {
              iconTheme: { primary: '#ffffff', secondary: 'rgb(var(--color-primary-rgb))' },
              style: {
                background: 'rgb(var(--color-primary-rgb))',
                color: '#ffffff',
                border: '1px solid rgb(var(--color-primary-dark-rgb))',
              },
            },
          }}
        />
      </BrowserRouter>
    </QueryClientProvider>
  </React.StrictMode>,
)
