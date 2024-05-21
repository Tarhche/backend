<script setup lang="ts">
const props = defineProps(['data'])
const emit = defineEmits(['result'])
function result(result:boolean){
  emit('result',result ,props.data.uuid)
}
</script>

<template>
<div class="modal-container w-100 h-100  d-flex justify-content-center">
  <div class="card px-3 py-2">
    <div class="card-body">
      <p>
        آیا میخواهید مقاله
        <span class="text-danger">{{ data.title }}</span>
        حذف شود ؟
      </p>
      <p> با حذف آن ، مقاله دیگر قابل دسترس و بازگردانی نخواهد بود ! </p>
    </div>
    <div class="card-footer bg-transparent border-0 d-flex flex-column gap-2 gap-md-0 flex-md-row justify-content-md-between ">
      <button class="btn btn-sm  btn-outline-success" @click ='result(false)' > نه پشیمون شدم.</button>
      <button class="btn btn-sm  btn-outline-danger" @click ='result(true)'>بله حذفش کن.</button>
    </div>
  </div>
</div>
</template>

<style scoped lang="scss">
.modal-container{
  position: fixed;
  inset: 0;
  background: rgba(0,0,0,0.3);
  backdrop-filter: blur(3px);
  z-index: 5;
  .card{
    margin-top: 10vh;
    height: fit-content;
    p:first-child{
      font-size: 15px;
      span{
        font-size: 16px;
        font-weight: bold;
      }
    }
    p:last-child{
      font-size: 12px;
    }
  }
}
</style>