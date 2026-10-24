import axios from 'axios'
import useApiTimingStore from '../store/apiTimingStore'

const baseURL = import.meta.env.VITE_API_URL || 'http://localhost:8800/api/v1'

const client = axios.create({
  baseURL,
  withCredentials: true,
  headers: { 'Content-Type': 'application/json' },
})

function getCookie(name) {
  const match = document.cookie.match(new RegExp('(^| )' + name + '=([^;]+)'))
  return match ? match[2] : null
}

client.interceptors.request.use((config) => {
  const token = getCookie('csrf_token')
  if (token) {
    config.headers['X-CSRF-Token'] = token
  }
  config.metadata = { startTime: performance.now() }
  return config
})

function recordDuration(config) {
  if (!config?.metadata) return
  useApiTimingStore.getState().setLastDuration(Math.round(performance.now() - config.metadata.startTime))
}

client.interceptors.response.use(
  (response) => {
    recordDuration(response.config)
    return response
  },
  (error) => {
    recordDuration(error.config)
    const status = error.response?.status
    if (status === 401 && window.location.pathname !== '/login') {
      window.location.href = '/login'
    } else if (status === 503) {
      window.location.href = '/maintenance'
    }
    return Promise.reject(error)
  },
)

export default client
