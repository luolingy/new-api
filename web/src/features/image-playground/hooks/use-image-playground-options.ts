/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'

import { getUserGroups, getUserModels } from '@/features/playground/api'

type UseImagePlaygroundOptionsParams = {
  group: string
  allowedModels: string[]
}

export function useImagePlaygroundOptions(
  params: UseImagePlaygroundOptionsParams
) {
  const groupsQuery = useQuery({
    queryKey: ['image-playground-groups'],
    queryFn: getUserGroups,
  })

  const modelsQuery = useQuery({
    queryKey: ['image-playground-models', params.group],
    queryFn: () => getUserModels(params.group),
    enabled: params.group !== '',
  })

  const models = useMemo(() => {
    const available = modelsQuery.data ?? []
    if (params.allowedModels.length === 0) return available
    return available.filter((model) =>
      params.allowedModels.includes(model.value)
    )
  }, [modelsQuery.data, params.allowedModels])

  return {
    groups: groupsQuery.data ?? [],
    models,
    isLoading: groupsQuery.isLoading || modelsQuery.isLoading,
  }
}
