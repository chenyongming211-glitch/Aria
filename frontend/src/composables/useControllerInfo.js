import api from './useApi'

export const fetchControllerInfo = async () => {
  const response = await api.get('/v2/controller-info')
  return response.data?.data || response.data || {}
}
