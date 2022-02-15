import { ApiGetTradingRankByID, ITradingRank } from '@/apis/trading'
import { changeTheme } from '@/assets/ts/tools'
import { BackHeader } from '@/components/BackHeader'
import { FullLoading } from '@/components/Loading'
import { get$t } from '@/locales/tools'
import { $get } from '@/stores/localStorage'
import BigNumber from 'bignumber.js'
import React, { useEffect, useState } from 'react'
import { useIntl, useParams } from 'umi'
import styles from './rank.less'

export default function Page() {
  const { id } = useParams<{ id: string }>()
  const [list, setList] = useState<ITradingRank[]>([])
  const [symbol, setSymbol] = useState("")
  const [amount, setAmount] = useState("0")
  const [loaded, setLoaded] = useState(false)
  const [userID, setUserID] = useState("")
  const $t = get$t(useIntl())
  useEffect(() => {
    initPage().then(() => {
      changeTheme('#D75150')
      let body = document.getElementsByTagName("body")[0]
      body.style.backgroundColor = "#B5312F"
    })
    return () => {
      changeTheme('#fff')
    }
  }, [])
  const initPage = async () => {
    const data = await ApiGetTradingRankByID(id)
    for (let i = 0; i < 10; i++)
      data.list[i] = data.list[i] || undefined
    setList(data.list)
    setSymbol(data.symbol)
    setAmount(data.amount)
    setUserID($get("user").user_id)
    setLoaded(true)
  }
  const idx = list.findIndex(item => item?.user_id === userID)
  return (
    <div className={`safe-view ${styles.container}`}>
      <BackHeader name={$t('trading.rank')} isWhite />

      <div className={styles.head}>
        <div className={styles.headItem2}>
          <img className={styles.avatar} src={getValueByItem('avatar', symbol, list[1])} alt="" />
        </div>
        <div className={styles.headItem1}>
          <img className={styles.first} src={require('@/assets/img/active/trading/first.png')} alt="" />
          <img className={styles.avatar} src={getValueByItem('avatar', symbol, list[0])} alt="" />
        </div>
        <div className={styles.headItem3}>
          <img className={styles.avatar} src={getValueByItem('avatar', symbol, list[2])} alt="" />
        </div>
      </div>

      <div className={styles.headInfo}>
        <div className={styles.headInfoItem}>
          <div className={styles.headInfoItemName}>{getValueByItem('full_name', symbol, list[1])}</div>
          <div className={styles.headInfoItemAmount}>{getValueByItem('amount', symbol, list[1])}</div>
        </div>
        <div className={styles.headInfoItem}>
          <div className={styles.headInfoItemName}>{getValueByItem('full_name', symbol, list[0])}</div>
          <div className={styles.headInfoItemAmount}>{getValueByItem('amount', symbol, list[0])}</div>
        </div>
        <div className={styles.headInfoItem}>
          <div className={styles.headInfoItemName}>{getValueByItem('full_name', symbol, list[2])}</div>
          <div className={styles.headInfoItemAmount}>{getValueByItem('amount', symbol, list[2])}</div>
        </div>
      </div>
      <div className={styles.list}>
        {list.slice(3).map((v, i) => <div key={i} className={styles.item}>
          <img className={styles.itemAvatar} src={getValueByItem('avatar', symbol, v)} alt="" />
          <p className={styles.itemAmount}>{getValueByItem('amount', symbol, v)}</p>
          <p className={styles.itemID}>{getValueByItem('identity_number', symbol, v)}</p>
          <p className={styles.itemRank}>{$t('trading.ranked1', { i: i + 4 })}</p>
        </div>)}
        {/* <div className={styles.item}>
          <img className={styles.itemAvatar} src={test} alt="" />
          <p className={styles.itemAmount}>10,000 PINK</p>
          <p className={styles.itemID}>7****2</p>
          <p className={styles.itemRank}>第 5 名</p>
        </div> */}
      </div>
      <p className={styles.tips}>{$t('trading.amount', { amount, symbol })}，{
        idx === -1 ? $t('trading.noRank') : $t('trading.ranked2', { i: idx + 1 })
      }</p>
      {!loaded && <FullLoading mask />}
    </div>
  )
}

const formatAmount = (amount: string) => {
  const n = new BigNumber(amount)
  if (n.isLessThanOrEqualTo(10000)) return n.toFormat()
  if (n.isLessThanOrEqualTo(10000 * 10000)) return new BigNumber(n.dividedBy(10000).toFixed(2)).toFormat() + '万'
  return new BigNumber(n.dividedBy(10000 * 10000).toFixed(2)).toFormat() + '亿'
}

const getValueByItem = (type: string, symbol: string, item?: ITradingRank) => {
  switch (type) {
    case "avatar":
      return item ? item.avatar : require('@/assets/img/active/trading/default.png')
    case "full_name":
      return item ? item.full_name : '？？？'
    case "identity_number":
      return item ? item.identity_number : '？？？'
    case "amount":
      return (item ? formatAmount(item.amount) : '0') + ' ' + symbol
  }
}

