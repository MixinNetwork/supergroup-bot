import { BackHeader } from '@/components/BackHeader'
import React from 'react'
import styles from './index.less'

export default function Page() {
  return (
    <div>
      <BackHeader name="春节活动" isWhite={false} />
      <h1 className={styles.title}>Page home/activity/card/index</h1>
    </div>
  )
}
