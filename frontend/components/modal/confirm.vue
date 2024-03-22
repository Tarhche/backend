<script setup lang="ts">
const emit = defineEmits(['close' , 'confirm'])

function cancel() {
  emit("close")
}

function confirm() {
  emit("confirm")
}
</script>

<template>
  <div class="backdrop-confirm-modal" @click.self="cancel">
    <div class="confirm-modal mx-auto">
      <div class="card rounded ">
        <div class="w-100 d-flex p-2 close" @click="cancel"><i class="fa-solid fa-close me-auto"></i></div>
        <div class="card-body d-flex flex-column flex-md-row align-items-baseline gap-1 pb-0">
          <p class="card-title"> آیا از حذف این پست  اطمینان دارید ؟</p>
          <p class="card-title text-muted"> ( با حذف آن دیگر قابل دسترس نیست ) </p>
        </div>
        <div class="card-footer d-flex flex-column flex-md-row gap-2 justify-content-between border-0 bg-white">
          <button class="btn btn-outline-success btn-sm w-100" @click="cancel"> نه پشمون شدم</button>
          <button class="btn btn-outline-danger btn-sm w-100" @click="confirm">بله حذف کن</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.backdrop-confirm-modal {
  position: fixed;
  top: 0;
  right: 0;
  z-index: 10;
  backdrop-filter: blur(5px);
  width: 100%;
  height: 100%;
  background-color: rgba(176, 176, 176, 0.37);
}

.confirm-modal {
  margin-top: 15vh;
  max-width: 50%;

}

.card {
  box-shadow: 0 0 5px 1px rgba(0, 0, 0, 0.32);
  scale: 0;
  animation:scale ease 0.5s forwards 1s ;
}
@keyframes scale {
  from{
    scale: 0;
    opacity: 0;
  }
  to{
    scale: 1;
    opacity: 1;
  }

}
.text-muted {
  font-size: 0.8rem;
}
.close{
  cursor: pointer;
}
</style>