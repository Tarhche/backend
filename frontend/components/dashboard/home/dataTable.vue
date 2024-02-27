<script setup lang="ts">
const props = defineProps(['data'])
</script>

<template>
  <div class="table-responsive m-b-40">
    <table class="table  table-striped table-borderless table-data3">
      <thead class="table-dark text-center">
      <tr>
        <th>تاریخ</th>
        <th>نوع</th>
        <th>توضیحات</th>
        <th>وضعیت</th>
        <th>قیمت</th>
      </tr>
      </thead>
      <tbody class="table-group-divider text-center">
      <tr v-for="(item , index) in props.data.dataTable" :key="index">
        <td>{{ item.date }}</td>
        <td>{{ item.device }}</td>
        <td>{{ item.deviceName }}</td>
        <td :class="{process:item.success === 'Processed' ,denied:item.success === 'Denied' }">
          {{ item.success }}
        </td>
        <td>{{ item.price }}</td>
      </tr>
      </tbody>
    </table>
  </div>
</template>

<style scoped>

</style>