import { apis } from './http'

export interface IMint {
  asset_id: string
  created_at: string
  daily_amount: string
  daily_end: string
  daily_time: string
  description: string
  extra_asset_id: string
  extra_daily_amount: string
  extra_first_amount: string
  first_amount: string
  first_end: string
  first_time: string
  mining_id: string
  reward_asset_id: string
  status: string
  symbol: string
  title: string
  faq: string
  join_tips: string
  join_url: string
}

export interface IMintRecord {
  record_id: string
  status: number
  date: string
  profit: string
  amount: string
  symbol: string
}

export const ApiGetMintByID = (id: string): Promise<IMint> => apis.get(`/mint/${id}`)

export const ApiPostMintByID = (record_id: string): Promise<IMint> => apis.post(`/mint`, { record_id })

export const ApiGetMintRecord = (mint_id: string): Promise<IMintRecord[]> => apis.get(`/mint/record`, { mint_id })